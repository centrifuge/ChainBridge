package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ChainSafe/ChainBridge/bindings/Bridge"
	"github.com/ChainSafe/ChainBridge/bindings/ERC20Handler"
	"github.com/ChainSafe/ChainBridge/chains/ethereum"
	"github.com/ChainSafe/ChainBridge/config"
	utils "github.com/ChainSafe/ChainBridge/shared/ethereum"
	"github.com/centrifuge/chainbridge-utils/core"
	"github.com/centrifuge/chainbridge-utils/msg"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types/codec"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"math/big"
	"strconv"

	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli/v2"
	"io"
	"net/http"
)

type FungibleTransferEvent struct {
	Nonce  uint64
	Amount types.U256
	Dest   string
	Sender string
	Failed bool
	Status uint8
}

type ProposalSucceededEvent struct {
	Nonce   uint64
	ChainId uint8
}

func runMonitor() cli.ActionFunc {
	return func(ctx *cli.Context) error {
		cfg, err := config.GetConfig(ctx)
		if err != nil {
			panic(err)
		}

		chain := cfg.Chains[0]
		chainId, err := strconv.Atoi(chain.Id)
		if err != nil {
			panic(err)
		}
		ethCoreChainConfig := &core.ChainConfig{
			Name:           chain.Name,
			Id:             msg.ChainId(chainId),
			Endpoint:       chain.Endpoint,
			From:           chain.From,
			KeystorePath:   cfg.KeystorePath,
			Insecure:       false,
			BlockstorePath: ctx.String(config.BlockstorePathFlag.Name),
			FreshStart:     ctx.Bool(config.FreshStartFlag.Name),
			LatestBlock:    ctx.Bool(config.LatestBlockFlag.Name),
			Opts:           chain.Opts,
		}
		sysErr := make(chan error)
		logger := log.Root().New("chain", ethCoreChainConfig.Name)
		ethChain, err := ethereum.InitializeChain(ethCoreChainConfig, logger, sysErr, nil)
		if err != nil {
			panic(err)
		}

		bridgeContract := ethChain.GetBridgeContract()

		log.Info("Parsing CFG -> wCFG Transactions")
		totalOutAmount := big.NewInt(0)
		totalFailedOutAmount := big.NewInt(0)
		startPage := 0
		for {
			events, err := parseCFGToWCFGEvents(ethChain, bridgeContract, ethChain.GetERC20HandlerAddress(), startPage)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				break
			}

			for i := 0; i < len(events); i++ {
				if events[i].Failed {
					log.Error("Failed CFG -> wCFG tx", "nonce", events[i].Nonce, "status", events[i].Status, "amount", events[i].Amount, "recipient", events[i].Dest)
					totalFailedOutAmount.Add(totalFailedOutAmount, events[i].Amount.Int)
				}
				totalOutAmount.Add(totalOutAmount, events[i].Amount.Int)
			}
			startPage++
		}
		log.Info("Parsed CFG -> wCFG", "total_amount", totalOutAmount, "failed_amount", totalFailedOutAmount)

		log.Info("Parsing wCFG -> CFG Transactions")
		events, err := parseWCFGToCFGEvents(ethChain, bridgeContract, ethChain.GetERC20HandlerContract())
		if err != nil {
			panic(err)
		}

		totalInAmount := big.NewInt(0)
		totalFailedInAmount := big.NewInt(0)
		for i := 0; i < len(events); i++ {
			if events[i].Failed {
				log.Error("Failed wCFG -> CFG tx", "nonce", events[i].Nonce, "sender", events[i].Sender, "amount", events[i].Amount, "recipient", events[i].Dest)
				totalFailedInAmount.Add(totalFailedInAmount, events[i].Amount.Int)
			}
			totalInAmount.Add(totalInAmount, events[i].Amount.Int)
		}
		log.Info("Parsed wCFG -> CFG", "total_amount", totalInAmount, "failed_amount", totalFailedInAmount)

		return nil
	}
}

func parseWCFGToCFGEvents(ethChain *ethereum.Chain, bridgeContract *Bridge.Bridge, erc20Handler *ERC20Handler.ERC20Handler) ([]FungibleTransferEvent, error) {
	depTotalCount, err := bridgeContract.DepositCounts(ethChain.GetCallOpts(), 1)
	if err != nil {
		return nil, err
	}
	log.Info("Total Deposit Count", "count", depTotalCount)

	log.Info("Getting all succeeded proposal events")
	var succeededNonces []uint64
	startPage := 0
	for {
		events, err := parseSucceededProposalEvents(startPage)
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			break
		}

		for i := 0; i < len(events); i++ {
			succeededNonces = append(succeededNonces, events[i].Nonce)
		}
		startPage++
	}

	var events []FungibleTransferEvent
	for i := depTotalCount; i > 0; i-- {
		//log.Info("Parsing Deposit Count", "count", i)
		depRecord, err := erc20Handler.DepositRecords(ethChain.GetCallOpts(), 1, i)
		if err != nil {
			return nil, err
		}
		if depRecord.Amount == nil {
			log.Warn("ETH Contract in inconsistent state - should not happen", "nonce", i)
			continue
		}
		//log.Info("Deposit Record", "nonce", i, "amount", depRecord.Amount.String())

		events = append(events, FungibleTransferEvent{
			Nonce:  i,
			Amount: types.NewU256(*depRecord.Amount),
			Dest:   hexutil.Encode(depRecord.DestinationRecipientAddress),
			Sender: depRecord.Depositer.Hex(),
			Failed: !Contains(succeededNonces, i),
		})

	}

	return events, nil
}

func parseSucceededProposalEvents(startPage int) ([]ProposalSucceededEvent, error) {
	payload := map[string]interface{}{
		"row":    100,
		"page":   startPage,
		"module": "chainbridge",
		"call":   "proposalsucceeded",
	}

	var resp struct {
		Code int `json:"code"` // must be zero for success
		Data struct {
			Count  int `json:"count"` // must be more than 0
			Events []struct {
				BlockNum int    `json:"block_num"`
				Params   string `json:"params"`
			} `json:"events"`
		} `json:"data"`
	}

	err := makeCall("POST", fmt.Sprintf("https://%s.webapi.subscan.io/api/scan/events", "centrifuge"), payload, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Code != 0 || resp.Data.Count < 1 {
		return nil, fmt.Errorf("failed to get stuff", resp.Code)
	}

	var events []ProposalSucceededEvent
	for i := 0; i < len(resp.Data.Events); i++ {
		var eventParams []map[string]interface{}
		err = json.Unmarshal([]byte(resp.Data.Events[i].Params), &eventParams)
		if err != nil {
			return nil, err
		}
		events = append(events, ProposalSucceededEvent{
			Nonce:   uint64(eventParams[1]["value"].(float64)),
			ChainId: uint8(eventParams[0]["value"].(float64)),
		})
	}

	return events, nil
}

func parseCFGToWCFGEvents(ethChain *ethereum.Chain, bridgeContract *Bridge.Bridge, erc20HandlerAddress common.Address, startPage int) ([]FungibleTransferEvent, error) {
	payload := map[string]interface{}{
		"row":    100,
		"page":   startPage,
		"module": "chainbridge",
		"call":   "fungibletransfer",
	}

	var resp struct {
		Code int `json:"code"` // must be zero for success
		Data struct {
			Count  int `json:"count"` // must be more than 0
			Events []struct {
				BlockNum int    `json:"block_num"`
				Params   string `json:"params"`
			} `json:"events"`
		} `json:"data"`
	}

	err := makeCall("POST", fmt.Sprintf("https://%s.webapi.subscan.io/api/scan/events", "centrifuge"), payload, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Code != 0 || resp.Data.Count < 1 {
		return nil, fmt.Errorf("failed to get stuff", resp.Code)
	}

	latestBlock, err := ethChain.GetLatestHeaderBlock()
	if err != nil {
		return nil, err
	}

	var events []FungibleTransferEvent
	for i := 0; i < len(resp.Data.Events); i++ {
		var eventParams []map[string]interface{}
		err = json.Unmarshal([]byte(resp.Data.Events[i].Params), &eventParams)
		if err != nil {
			return nil, err
		}

		propNonce := uint64(eventParams[1]["value"].(float64))
		propAmount := eventParams[3]["value"].(string)
		propDest := eventParams[4]["value"].(string)
		if !has0xPrefix(propDest) {
			propDest = fmt.Sprintf("0x%s", propDest)
		}
		propSender := "GATHER_ME_FROM_EXTRINSIC"

		var propAmountInt types.U256
		propAmountBytes, err := hex.DecodeString(propAmount[2:])
		if err != nil {
			return nil, err
		}
		err = codec.Decode(propAmountBytes, &propAmountInt)
		if err != nil {
			return nil, err
		}
		//log.Debug("Parsed Event", "Nonce", propNonce, "Amount", propAmountInt.String(), "Dest", propDest, "Sender", propSender)

		ethProp, err := bridgeContract.GetProposal(ethChain.GetCallOpts(), 1, propNonce,
			calcDataHash(propAmountInt.Bytes(), hexutil.MustDecode(propDest), erc20HandlerAddress.Bytes()))
		if err != nil {
			return nil, err
		}

		//log.Info("EthProp", "nonce", propNonce, "status", ethProp.Status, "block", ethProp.ProposedBlock.String())

		events = append(events, FungibleTransferEvent{
			Nonce:  propNonce,
			Amount: propAmountInt,
			Dest:   propDest,
			Sender: propSender,
			Failed: ethProp.Status == 4 || (ethProp.Status == 1 && (big.NewInt(0).Sub(latestBlock, ethProp.ProposedBlock).Cmp(big.NewInt(100)) > 0)),
			Status: ethProp.Status,
		})
	}

	return events, nil

}

func calcDataHash(amount, recipient, erc20Handler []byte) [32]byte {
	data := ethereum.ConstructErc20ProposalData(amount, recipient)
	return utils.Hash(append(erc20Handler, data...))
}

func makeCall(method, url string, body interface{}, resp interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		err := call(method, url, bytes.NewReader(data), resp)
		if err != nil {
			continue
		}

		break
	}

	return nil
}

func call(method, url string, body io.Reader, resp interface{}) error {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authority", "centrifuge.webapi.subscan.io")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/93.0.4577.63 Safari/537.36")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Sec-Gpc", "1")
	req.Header.Set("Origin", "https://centrifuge.subscan.io")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Referer", "https://centrifuge.subscan.io/")

	respBody, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer respBody.Body.Close()
	if respBody.StatusCode != 200 {
		return fmt.Errorf("failed to make api call: %s-%s", method, url)
	}

	data, err := io.ReadAll(respBody.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, resp)
}

func Contains(list []uint64, x uint64) bool {
	for _, item := range list {
		if item == x {
			return true
		}
	}
	return false
}

func has0xPrefix(input string) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

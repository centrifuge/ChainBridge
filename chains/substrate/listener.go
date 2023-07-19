// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/centrifuge/go-substrate-rpc-client/v4/registry/parser"
	"math/big"
	"time"

	"github.com/ChainSafe/ChainBridge/chains"
	"github.com/ChainSafe/log15"
	"github.com/centrifuge/chainbridge-utils/blockstore"
	metrics "github.com/centrifuge/chainbridge-utils/metrics/types"
	"github.com/centrifuge/chainbridge-utils/msg"
	"github.com/centrifuge/go-substrate-rpc-client/v4/registry/retriever"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type listener struct {
	name           string
	chainId        msg.ChainId
	startBlock     uint64
	blockstore     blockstore.Blockstorer
	conn           *Connection
	subscriptions  map[eventName]eventHandler // Handlers for specific events
	router         chains.Router
	log            log15.Logger
	stop           <-chan int
	sysErr         chan<- error
	latestBlock    metrics.LatestBlock
	metrics        *metrics.ChainMetrics
	eventRetriever retriever.EventRetriever
}

// Frequency of polling for a new block
var BlockRetryInterval = time.Second * 5
var BlockRetryLimit = 5

func NewListener(
	conn *Connection,
	name string,
	id msg.ChainId,
	startBlock uint64,
	log log15.Logger,
	bs blockstore.Blockstorer,
	stop <-chan int,
	sysErr chan<- error,
	m *metrics.ChainMetrics,
	eventRetriever retriever.EventRetriever,
) *listener {
	return &listener{
		name:           name,
		chainId:        id,
		startBlock:     startBlock,
		blockstore:     bs,
		conn:           conn,
		subscriptions:  make(map[eventName]eventHandler),
		log:            log,
		stop:           stop,
		sysErr:         sysErr,
		latestBlock:    metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:        m,
		eventRetriever: eventRetriever,
	}
}

func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start creates the initial subscription for all events
func (l *listener) start() error {
	// Check whether latest is less than starting block
	header, err := l.conn.api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return err
	}
	if uint64(header.Number) < l.startBlock {
		return fmt.Errorf("starting block (%d) is greater than latest known block (%d)", l.startBlock, header.Number)
	}

	for _, sub := range Subscriptions {
		err := l.registerEventHandler(sub.name, sub.handler)
		if err != nil {
			return err
		}
	}

	go func() {
		err := l.pollBlocks()
		if err != nil {
			l.log.Error("Polling blocks failed", "err", err)
		}
	}()

	return nil
}

// registerEventHandler enables a handler for a given event. This cannot be used after Start is called.
func (l *listener) registerEventHandler(name eventName, handler eventHandler) error {
	if l.subscriptions[name] != nil {
		return fmt.Errorf("event %s already registered", name)
	}
	l.subscriptions[name] = handler
	return nil
}

var ErrBlockNotReady = errors.New("required result to be 32 bytes, but got 0")

// pollBlocks will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `l.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to BlockRetryLimit times before returning with an error.
func (l *listener) pollBlocks() error {
	l.log.Info("Polling Blocks...")
	var currentBlock = l.startBlock
	var retry = BlockRetryLimit
	for {
		select {
		case <-l.stop:
			return errors.New("polling terminated")
		default:
			// No more retries, goto next block
			if retry == 0 {
				l.sysErr <- fmt.Errorf("event polling retries exceeded (chain=%d, name=%s)", l.chainId, l.name)
				return nil
			}

			// Get finalized block hash
			finalizedHash, err := l.conn.api.RPC.Chain.GetFinalizedHead()
			if err != nil {
				l.log.Error("Failed to fetch finalized hash", "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get finalized block header
			finalizedHeader, err := l.conn.api.RPC.Chain.GetHeader(finalizedHash)
			if err != nil {
				l.log.Error("Failed to fetch finalized header", "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			if l.metrics != nil {
				l.metrics.LatestKnownBlock.Set(float64(finalizedHeader.Number))
			}

			// Sleep if the block we want comes after the most recently finalized block
			if currentBlock > uint64(finalizedHeader.Number) {
				l.log.Debug("Block not yet finalized", "target", currentBlock, "latest", finalizedHeader.Number)
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get hash for latest block, sleep and retry if not ready
			hash, err := l.conn.api.RPC.Chain.GetBlockHash(currentBlock)
			if err != nil && err.Error() == ErrBlockNotReady.Error() {
				time.Sleep(BlockRetryInterval)
				continue
			} else if err != nil {
				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			l.log.Debug("Querying block for deposit events", "target", currentBlock)

			err = l.processEvents(hash)
			if err != nil {
				l.log.Error("Failed to process events in block", "block", currentBlock, "err", err)
				retry--
				continue
			}

			// Write to blockstore
			err = l.blockstore.StoreBlock(big.NewInt(0).SetUint64(currentBlock))
			if err != nil {
				l.log.Error("Failed to write to blockstore", "err", err)
			}

			if l.metrics != nil {
				l.metrics.BlocksProcessed.Inc()
				l.metrics.LatestProcessedBlock.Set(float64(currentBlock))
			}

			currentBlock++
			l.latestBlock.Height = big.NewInt(0).SetUint64(currentBlock)
			l.latestBlock.LastUpdated = time.Now()
			retry = BlockRetryLimit
		}
	}
}

// processEvents fetches a block and parses out the events, calling Listener.handleEvents()
func (l *listener) processEvents(hash types.Hash) error {
	l.log.Trace("Fetching events for block", "hash", hash.Hex())

	events, err := l.eventRetriever.GetEvents(hash)

	if err != nil {
		return fmt.Errorf("event retrieving error: %w", err)
	}

	l.handleEvents(events)
	l.log.Trace("Finished processing events", "block", hash.Hex())

	return nil
}

const MetadataUpdateEvent = "ParachainSystem.ValidationFunctionApplied"

// handleEvents calls the associated handler for all registered event types
func (l *listener) handleEvents(events []*parser.Event) {
	for _, event := range events {
		switch {
		case l.subscriptions[FungibleTransfer] != nil && event.Name == string(FungibleTransfer):
			l.log.Debug("Handling FungibleTransfer event")
			l.submitMessage(l.subscriptions[FungibleTransfer](event.Fields, l.log))
		case l.subscriptions[NonFungibleTransfer] != nil && event.Name == string(NonFungibleTransfer):
			l.log.Debug("Handling NonFungibleTransfer event")
			l.submitMessage(l.subscriptions[NonFungibleTransfer](event.Fields, l.log))
		case l.subscriptions[GenericTransfer] != nil && event.Name == string(GenericTransfer):
			l.log.Debug("Handling GenericTransfer event")
			l.submitMessage(l.subscriptions[GenericTransfer](event.Fields, l.log))
		case event.Name == MetadataUpdateEvent:
			l.log.Debug("Received metadata update event")

			if err := l.conn.updateMetadata(); err != nil {
				l.log.Error("Unable to update metadata", "error", err)
			}
		}
	}

}

// submitMessage inserts the chainId into the msg and sends it to the router
func (l *listener) submitMessage(m msg.Message, err error) {
	if err != nil {
		log15.Error("Critical error processing event", "err", err)
		return
	}
	m.Source = l.chainId
	err = l.router.Send(m)
	if err != nil {
		log15.Error("failed to process event", "err", err)
	}
}

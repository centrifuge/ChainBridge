module github.com/ChainSafe/ChainBridge

go 1.18

// NOTE - this is a specific branch - https://github.com/centrifuge/go-substrate-rpc-client/tree/remove-claims-event,
// that does not have the `Claims_Claimed` event since it is colliding with the one that we have in the claims pallet
// of Centrifuge chain.
require github.com/centrifuge/go-substrate-rpc-client/v4 v4.0.13-0.20230111181438-6501f611f49f

require (
	github.com/ChainSafe/log15 v1.0.0
	github.com/centrifuge/chain-custom-types v0.0.0-20220323235722-1cdf9a3ad7f1
	github.com/centrifuge/chainbridge-substrate-events v0.0.0-20220215222726-8c1d3a5cad10
	github.com/centrifuge/chainbridge-utils v1.1.1-0.20221001051926-ecac2af5cb68
	github.com/prometheus/client_golang v1.4.1
	github.com/stretchr/testify v1.8.2
	github.com/urfave/cli/v2 v2.3.0
	golang.org/x/crypto v0.0.0-20220926161630-eccd6366d1be
)

require (
	github.com/ChainSafe/go-schnorrkel v1.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/cosmos/go-bip39 v1.0.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0-20190314233015-f79a8a8ca69d // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/deckarep/golang-set v1.8.0 // indirect
	github.com/decred/base58 v1.0.3 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/google/go-cmp v0.5.4 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/gtank/merlin v0.1.1 // indirect
	github.com/gtank/ristretto255 v0.1.2 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mimoo/StrobeGo v0.0.0-20210601165009-122bf33a46e0 // indirect
	github.com/pierrec/xxHash v0.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.9.1 // indirect
	github.com/prometheus/procfs v0.0.10 // indirect
	github.com/rs/cors v1.8.2 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/vedhavyas/go-subkey v1.0.3 // indirect
	golang.org/x/sys v0.0.0-20220928140112-f11e5e49a4ec // indirect
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/protobuf v1.23.0 // indirect
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

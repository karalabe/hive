#!/bin/bash

# Startup script to initialize and boot a go-ethereum instance.
#
# This script assumes the following files:
#  - `geth` binary is located in the filesystem root
#  - `genesis.json` file is located in the filesystem root (mandatory)
#  - `chain.rlp` file is located in the filesystem root (optional)
#  - `blocks` folder is located in the filesystem root (optional)
#  - `keys` folder is located in the filesystem root (optional)
#
# This script assumes the following environment variables:
#
#  - HIVE_BOOTNODE                enode URL of the remote bootstrap node
#  - HIVE_NETWORK_ID              network ID number to use for the eth protocol
#  - HIVE_TESTNET                 whether testnet nonces (2^20) are needed
#  - HIVE_NODETYPE                sync and pruning selector (archive, full, light)
#
# Forks:
#
#  - HIVE_FORK_HOMESTEAD          block number of the homestead hard-fork transition
#  - HIVE_FORK_DAO_BLOCK          block number of the DAO hard-fork transition
#  - HIVE_FORK_DAO_VOTE           whether the node support (or opposes) the DAO fork
#  - HIVE_FORK_TANGERINE          block number of Tangerine Whistle transition
#  - HIVE_FORK_SPURIOUS           block number of Spurious Dragon transition
#  - HIVE_FORK_BYZANTIUM          block number for Byzantium transition
#  - HIVE_FORK_CONSTANTINOPLE     block number for Constantinople transition
#  - HIVE_FORK_PETERSBURG         block number for ConstantinopleFix/PetersBurg transition
#  - HIVE_FORK_ISTANBUL           block number for Istanbul transition
#  - HIVE_FORK_MUIRGLACIER        block number for Muir Glacier transition
#  - HIVE_FORK_BERLIN             block number for Berlin transition
#  - HIVE_FORK_LONDON             block number for London
#
# Clique PoA:
#
#  - HIVE_CLIQUE_PERIOD           enables clique support. value is block time in seconds.
#  - HIVE_CLIQUE_PRIVATEKEY       private key for clique mining
#
# Other:
#
#  - HIVE_MINER                   enable mining. value is coinbase address.
#  - HIVE_MINER_EXTRA             extra-data field to set for newly minted blocks
#  - HIVE_SKIP_POW                if set, skip PoW verification during block import
#  - HIVE_LOGLEVEL		            client loglevel (0-5)
#  - HIVE_GRAPHQL_ENABLED         enables graphql on port 8545
#  - HIVE_LIGHTSERVE              maximum percentage of time allowed for serving LES requests (0 for disabling)

# Immediately abort the script on any error encountered
set -e

geth=/usr/local/bin/geth
FLAGS="--pcscdpath=\"\""

if [ "$HIVE_LOGLEVEL" != "" ]; then
    FLAGS="$FLAGS --verbosity=$HIVE_LOGLEVEL"
fi

# It doesn't make sense to dial out, use only a pre-set bootnode.
FLAGS="$FLAGS --bootnodes=$HIVE_BOOTNODE"

if [ "$HIVE_SKIP_POW" != "" ]; then
	FLAGS="$FLAGS --fakepow"
fi

# If a specific network ID is requested, use that
if [ "$HIVE_NETWORK_ID" != "" ]; then
	FLAGS="$FLAGS --networkid $HIVE_NETWORK_ID"
else
    # Unless otherwise specified by hive, we try to avoid mainnet networkid. If geth detects mainnet network id,
    # then it tries to bump memory quite a lot
    FLAGS="$FLAGS --networkid 1337"
fi

# If the client is to be run in testnet mode, flag it as such
if [ "$HIVE_TESTNET" == "1" ]; then
	FLAGS="$FLAGS --testnet"
fi

# Handle any client mode or operation requests
if [ "$HIVE_NODETYPE" == "full" ]; then
	FLAGS="$FLAGS --syncmode fast "
fi
if [ "$HIVE_NODETYPE" == "light" ]; then
	FLAGS="$FLAGS --syncmode light "
fi

# Configure the chain.
mv /genesis.json /genesis-input.json
jq -f /mapper.jq /genesis-input.json > /genesis.json

# Dump genesis
echo "Supplied genesis state:"
cat /genesis.json

# Initialize the local testchain with the genesis state
echo "Initializing database with genesis state..."
$geth $FLAGS init /genesis.json

# Don't immediately abort, some imports are meant to fail
set +e

# Load the test chain if present
echo "Loading initial blockchain..."
if [ -f /chain.rlp ]; then
	$geth $FLAGS --gcmode=archive import /chain.rlp
else
	echo "Warning: chain.rlp not found."
fi

# Load the remainder of the test chain
echo "Loading remaining individual blocks..."
if [ -d /blocks ]; then
	(cd /blocks && $geth $FLAGS --gcmode=archive --verbosity=$HIVE_LOGLEVEL --nocompaction import `ls | sort -n`)
else
	echo "Warning: blocks folder not found."
fi

set -e

# Import clique signing key.
if [ "$HIVE_CLIQUE_PRIVATEKEY" != "" ]; then
    # Create password file.
    echo "Importing clique key..."
    echo "secret" > /geth-password-file.txt
    $geth --nousb account import --password /geth-password-file.txt <(echo "$HIVE_CLIQUE_PRIVATEKEY")

    # Ensure password file is used when running geth in mining mode.
    if [ "$HIVE_MINER" != "" ]; then
        FLAGS="$FLAGS --password /geth-password-file.txt --unlock $HIVE_MINER --allow-insecure-unlock"
    fi
fi

# Configure any mining operation
if [ "$HIVE_MINER" != "" ] && [ "$HIVE_NODETYPE" != "light" ]; then
	FLAGS="$FLAGS --mine --miner.threads 1 --miner.etherbase $HIVE_MINER"
fi
if [ "$HIVE_MINER_EXTRA" != "" ]; then
	FLAGS="$FLAGS --miner.extradata $HIVE_MINER_EXTRA"
fi
FLAGS="$FLAGS --miner.gasprice 16000000000"

# Configure any les flags
if [ "$HIVE_LIGHTSERVE" != "" ]; then
  FLAGS="$FLAGS --light.serve $HIVE_LIGHTSERVE --light.nosyncserve "
fi

# Configure RPC.
FLAGS="$FLAGS --http --http.addr=0.0.0.0 --http.port=8545 --http.api=admin,debug,eth,miner,net,personal,txpool,web3"
FLAGS="$FLAGS --ws --ws.addr=0.0.0.0 --ws.origins \"*\" --ws.api=admin,debug,eth,miner,net,personal,txpool,web3"
if [ "$HIVE_GRAPHQL_ENABLED" != "" ]; then
	FLAGS="$FLAGS --graphql"
fi
# used for the graphql to allow submission of unprotected tx
if [ "$HIVE_ALLOW_UNPROTECTED_TX" != "" ]; then
 	FLAGS="$FLAGS --rpc.allow-unprotected-txs"
fi

# Run the go-ethereum implementation with the requested flags.
FLAGS="$FLAGS --nat=none"
echo "Running go-ethereum with flags $FLAGS"
$geth $FLAGS

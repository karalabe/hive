package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/hive/hivesim"
)

var (
	// parameters used for signing transactions
	chainID  = big.NewInt(7)
	gasPrice = big.NewInt(30 * params.GWei)

	// would be nice to use a networkID that's different from chainID,
	// but some clients don't support the distinction properly.
	networkID = big.NewInt(7)
)

var clientEnv = hivesim.Params{
	"HIVE_NETWORK_ID":     networkID.String(),
	"HIVE_CHAIN_ID":       chainID.String(),
	"HIVE_FORK_HOMESTEAD": "0",
	"HIVE_FORK_TANGERINE": "0",
	"HIVE_FORK_SPURIOUS":  "0",
	// All tests use clique PoA to mine new blocks.
	"HIVE_CLIQUE_PERIOD":     "1",
	"HIVE_CLIQUE_PRIVATEKEY": "9c647b8b7c4e7c3490668fb6c11473619db80c93704c70893d3813af4090c39c",
	"HIVE_MINER":             "658bdf435d810c91414ec09147daa6db62406379",
}

var files = map[string]string{
	"/genesis.json": "./init/genesis.json",
}

type testSpec struct {
	Name  string
	About string
	Run   func(*TestEnv)
}

var tests = []testSpec{
	// HTTP RPC tests.
	{Name: "http/BalanceAndNonceAt", Run: balanceAndNonceAtTest},
	{Name: "http/CanonicalChain", Run: canonicalChainTest},
	{Name: "http/CodeAt", Run: CodeAtTest},
	{Name: "http/ContractDeployment", Run: deployContractTest},
	{Name: "http/ContractDeploymentOutOfGas", Run: deployContractOutOfGasTest},
	{Name: "http/EstimateGas", Run: estimateGasTest},
	{Name: "http/GenesisBlockByHash", Run: genesisBlockByHashTest},
	{Name: "http/GenesisBlockByNumber", Run: genesisBlockByNumberTest},
	{Name: "http/GenesisHeaderByHash", Run: genesisHeaderByHashTest},
	{Name: "http/GenesisHeaderByNumber", Run: genesisHeaderByNumberTest},
	{Name: "http/Receipt", Run: receiptTest},
	{Name: "http/SyncProgress", Run: syncProgressTest},
	{Name: "http/TransactionCount", Run: transactionCountTest},
	{Name: "http/TransactionInBlock", Run: transactionInBlockTest},
	{Name: "http/TransactionReceipt", Run: TransactionReceiptTest},

	// HTTP ABI tests.
	{Name: "http/ABICall", Run: callContractTest},
	{Name: "http/ABITransact", Run: transactContractTest},

	// WebSocket RPC tests.
	{Name: "ws/BalanceAndNonceAt", Run: balanceAndNonceAtTest},
	{Name: "ws/CanonicalChain", Run: canonicalChainTest},
	{Name: "ws/CodeAt", Run: CodeAtTest},
	{Name: "ws/ContractDeployment", Run: deployContractTest},
	{Name: "ws/ContractDeploymentOutOfGas", Run: deployContractOutOfGasTest},
	{Name: "ws/EstimateGas", Run: estimateGasTest},
	{Name: "ws/GenesisBlockByHash", Run: genesisBlockByHashTest},
	{Name: "ws/GenesisBlockByNumber", Run: genesisBlockByNumberTest},
	{Name: "ws/GenesisHeaderByHash", Run: genesisHeaderByHashTest},
	{Name: "ws/GenesisHeaderByNumber", Run: genesisHeaderByNumberTest},
	{Name: "ws/Receipt", Run: receiptTest},
	{Name: "ws/SyncProgress", Run: syncProgressTest},
	{Name: "ws/TransactionCount", Run: transactionCountTest},
	{Name: "ws/TransactionInBlock", Run: transactionInBlockTest},
	{Name: "ws/TransactionReceipt", Run: TransactionReceiptTest},

	// WebSocket subscription tests.
	{Name: "ws/NewHeadSubscription", Run: newHeadSubscriptionTest},
	{Name: "ws/LogSubscription", Run: logSubscriptionTest},
	{Name: "ws/TransactionInBlockSubscription", Run: transactionInBlockSubscriptionTest},

	// WebSocket ABI tests.
	{Name: "ws/ABICall", Run: callContractTest},
	{Name: "ws/ABITransact", Run: transactContractTest},
}

func main() {
	suite := hivesim.Suite{
		Name: "rpc",
		Description: `
The RPC test suite runs a set of RPC related tests against a running node. It tests
several real-world scenarios such as sending value transactions, deploying a contract or
interacting with one.`[1:],
	}
	// Add testing for general full nodes
	suite.Add(&hivesim.ClientTestSpec{
		Name:        "client launch",
		Description: `This test launches the client and collects its logs.`,
		Parameters:  clientEnv,
		Files:       files,
		Run:         runAllTests,
	})
	// Add testing les light client. This only works with geth for now.
	var (
		clients []string
		sim     = hivesim.New()
	)
	clientTypes, err := sim.ClientTypes()
	if err != nil {
		panic(err)
	}
	for _, name := range clientTypes {
		if !strings.HasPrefix(name, "go-ethereum") {
			continue
		}
		clients = append(clients, name)
	}
	for _, client := range clients {
		name := client
		suite.Add(hivesim.TestSpec{
			Name:        fmt.Sprintf("%s as LES server", name),
			Description: "This loads the test chain into the les server and verifies whether the light client can sync correctly.",
			Run:         func(t *hivesim.T) { runLESTests(t, name, clients) },
		})
	}
	hivesim.MustRunSuite(sim, suite)
}

func runLESTests(t *hivesim.T, serverType string, clientTypes []string) {
	// Start the LES server
	serverParams := clientEnv.Set("HIVE_NODETYPE", "full")
	serverParams = serverParams.Set("HIVE_LIGHTSERVE", "100")
	client := t.StartClient(serverType, serverParams, files)

	// Configure sink to connect to the source node.
	clientParams := clientEnv.Set("HIVE_NODETYPE", "light")
	enode, err := client.EnodeURL()
	if err != nil {
		t.Fatal("can't get node peer-to-peer endpoint:", enode)
	}

	// Sync all sink nodes against the source.
	for _, sinkType := range clientTypes {
		name := sinkType
		t.Run(hivesim.TestSpec{
			Name:        fmt.Sprintf("%s as LES server", name),
			Description: "This loads the test chain into the les server and verifies whether the light client can sync correctly.",
			Run: func(t *hivesim.T) {
				client := t.StartClient(name, clientParams, files)

				// todo(rjl493456442) Wait the initialization of light client
				time.Sleep(time.Second * 10)

				err := client.RPC().Call(nil, "admin_addPeer", enode)
				if err != nil {
					t.Fatalf("connection failed:", err)
				}
				runAllTests(t, client)
			},
		})
	}
}

// runAllTests runs the tests against a client instance.
// Most tests simply wait for tx inclusion in a block so we can run many tests concurrently.
func runAllTests(t *hivesim.T, c *hivesim.Client) {
	vault := newVault()

	s := newSemaphore(16)
	for _, test := range tests {
		test := test
		s.get()
		go func() {
			defer s.put()
			t.Run(hivesim.TestSpec{
				Name:        fmt.Sprintf("%s (%s)", test.Name, c.Type),
				Description: test.About,
				Run: func(t *hivesim.T) {
					switch test.Name[:strings.IndexByte(test.Name, '/')] {
					case "http":
						runHTTP(t, c, vault, test.Run)
					case "ws":
						runWS(t, c, vault, test.Run)
					default:
						panic("bad test prefix in name " + test.Name)
					}
				},
			})
		}()
	}
	s.drain()
}

type semaphore chan struct{}

func newSemaphore(n int) semaphore {
	s := make(semaphore, n)
	for i := 0; i < n; i++ {
		s <- struct{}{}
	}
	return s
}

func (s semaphore) get() { <-s }
func (s semaphore) put() { s <- struct{}{} }

func (s semaphore) drain() {
	for i := 0; i < cap(s); i++ {
		<-s
	}
}

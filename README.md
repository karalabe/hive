#  hive - Ethereum end-to-end test harness

hive is a testing rig designed to make it easy to run specification conformance and behavior tests against any eth1 client implementation.

# Public test results

An Ethereum Foundation server often runs Hive to check for consensus, p2p and blockchain compatibility as part of our CI workflow. This is needed to ensure a baseline quality for eth1 client implementations.

Test results are made public [here](https://hivetests.ethdevops.io/).

# Running hive

The `hive` project is based on Go. You'll need a valid Go (1.6 and upwards) installation available.

After cloning the repository,  build the hive executable by running `go build` inside the root directory.

Then, to run a simulation, use the following command:
```bash
./hive --sim <simulation> --client <client(s) you want to test against>  --loglevel <preferred log verbosity level>
```
For example, if you want to run the `discv4` test, here is how the command would look:

```bash
./hive --sim devp2p/discv4 --client go-ethereum_latest --loglevel 6
```

## Quickstart command lines

This section is a quick start guide for command line options covering typical hive usage scenarios.

### Consensus test
The following command will run consensus tests on the parity client
```text
   --sim consensus
   --client parity_latest
   --results-root /mytests/tests
```

### Devp2p tests

The following command will run devp2p tests on geth: 

```text
    --sim devp2p
    --client go-ethereum_latest
    --loglevel 6
    --results-root /mytests/tests
    --sim.parallelism 1
```
The `--sim.parallelism` flag will set the maximum number of clients against which to run the simulation in parallel. 

### Sync simulation

The following command will run a test verifying that a blockchain can be synced between differing implementations (in this case, parity and geth):

      --sim sync
      --client go-ethereum_latest,parity_latest
      --loglevel 6
      --results-root /mytests/tests

### Iterating on bug fixes locally

If you are testing locally and want to make changes to the simulation or client, run hive with the flag `--docker-nocache <simulation or client name>` so that hive rebuilds the container from scratch.

If you want to rebuild both, separate the names with a `,` as such: 
```text
--docker-nocache devp2p,go-ethereum_latest
```

# Adding a Simulation
There are two components to a simulation: 
1. a **simulation program written in Go** using the `hivesim` test API to coordinate the execution of your desired test; and
2. a **Dockerfile** to containerize both the simulation and the tests to be executed against client implementations.

## Placement
_(This section is relevant if you plan to merge your simulation upstream)_

If the theme of the test suite can be grouped in one of the directories located in `simulators/`, please place the new simulation in that directory. Otherwise, if the simulation cannot be categorized with the current groupings, create a new directory in `simulators/` and name it according to the theme of the test suite.

## Structure of a simulation
The purpose of the simulation is to coordinate the execution of your desired test by communicating with the hive server.

hive provides a `hivesim` test API that makes it relatively simple to communicate with the hive server in order to organize the docker containers and networks according to your test's needs.

###### Accessing the `hivesim` test API:
To access the `hivesim` API, create a `Suite` as such:
```go
	suite := hivesim.Suite{
		Name:        "MyTest",
		Description: "This simulation test does XYZ.",
	}
```
The `Suite` will only execute tests that are added to it using the `Add()` method. 

###### Adding a test to the `Suite`

To add a test to the `Suite`, you must write a function with the following signature:

```go
func myTestFunction(t *hivesim.T, c *hivesim.Client)
```
where 

* `hivesim.T` represents a running test. It behaves similarly to `testing.T` in package `testing` (a Golang standard library for testing), but has some additional methods for launching clients; and

* `hivesim.Client`represents a running client.

The job of `myTestFunction()` is to get any information necessary from the client container to execute the test and to create or modify docker networks in the case that the simulation requires a more complex networking set up.

`myTestFunction()` may be added to the `Suite` using `hivesim.ClientTestSpec`, which represents a test against a single client:

```go
type ClientTestSpec struct {
	Name        string // name of the test
	Description string // description of the test
	Parameters  Params // parameters necessary to configure the client
	Files       map[string]string // files to be mounted into the client container
	Run         func(*T, *Client) // the test function itself
}
```

where the `Run` field would take `myTestFunction` as the parameter.

######  Running the `Suite`

To run the `Suite`, you can call either `RunSuite()`or `MustRunSuite()` depending on how you want errors to be handled. 

* `RunSuite()` will run all tests in the `Suite`, returning an error upon failure. 

* `MustRunSuite()` runs the given suite, exiting the process if there is a problem executing the test.

Both Run functions take `Suite` as a parameter as well as a pointer to an instance of `hivesim.Simulation`, as such: 

```go
hivesim.MustRunSuite(hivesim.New(), suite)
```

`hivesim.Simulation` is just a wrapper API that can access the hive server. To get a new instance of `hivesim.Simulation`, call `hivesim.New()`. This will look up the hive host server URI and connect to it


###### Getting information about the client container

To get information about the client that is likely necessary for test execution, you can use `hivesim.Client`within the aforementioned test execution function (`myTestFunction`).

**Enode URL**

To get the client's enode URL, call the `EnodeURL()` method.

**RPC client**

Call `RPC()` to get an RPC client connected to the client's RPC server.

###### Configuring networks

The `hivesim.Simulation` API offers different ways to configure the network set-up for a test. To configure networks within the test execution function (`myTestFunction`), use the methods available on the `Sim` field of the `hivesim.T` parameter.

**Create a network**

```go
err := t.Sim.CreateNetwork(t.SuiteID, "networkName")
```

**Remove a network**

```go
err := t.Sim.RemoveNetwork(t.SuiteID, "networkName")
```

**Connect a container to a network**

```go
err := t.Sim.ConnectContainer(t.SuiteID, "networkName", c.Container)
```

where `c` is the `hivesim.Client` parameter of the test execution function.

If the simulation container also needs to be connected to a network, you can pass in the string "simulation" to the `ConnnectContainer` method, as such: 

```go
err := t.Sim.ConnectContainer(t.SuiteID, "networkName", "simulation")
```

**Disconnect a container from a network**

```go
err := t.Sim.DisconnectContainer(t.SuiteID, "networkName", c.Container)
```

**Get a container's IP address on a specific network**

```go
t.Sim.ContainerNetworkIP(t.SuiteID, "networkName", c.Container)
```

The default network used by hive is the `bridge` network. The client container's IP address on the bridge network is available as `IP` field of the `hivesim.Client` object.

However, in the case that the simulation container's IP address on the default network is needed, pass `"bridge"` in as the network name, as such: 

```go
t.Sim.ContainerNetworkIP(t.SuiteID, "bridge", "simulation")
```
## Dockerizing the Simulation
Create a Dockerfile and place it in the same directory as the simulation. The Dockerfile should: 

* build the simulation executable
* build the test tool / executable
* set the entrypoint as the simulation executable (this will ensure that the simulation handles the communication between the hive server and the test itself)

# Adding a Client

## Creating a client image

Adding a new client implementation to `hive` entails creating a Dockerfile (and related resources),
based on which `hive` will assemble the docker image to use as the blueprint for testing.

The client definition(s) should reside in the `clients` folder, inside a folder named `<project>` where `<project>` is the official name of the client (lowercase, no fancy characters). `hive` will automatically pick up all clients from this folder.

There aren't many contraints on the image itself, though a few required caveats exist:

 * It should be as tiny as possible (play nice with others). Preferably use `alpine` Linux.
 * It should expose the following ports: 8545 (HTTP RPC), 8546 (WS RPC), 30303 (devp2p).
 * It should have a single entrypoint (or script) defined, which can initialize and run the client.

For guidance, check out the reference [go-ethereum](https://github.com/ethereum/hive/blob/master/clients/go-ethereum/Dockerfile) client.

## Initializing the client

hive injects all the required configurations into the Linux containers prior to launching the client's `entrypoint` script. It is then left to this script to interpret all the environmental configs and initialize the client appropriately.

The chain configurations files:

 * `/genesis.json` contains the JSON specification of the Ethereum genesis states
 * `/chain.rlp` contains a batch of RLP encoded blocks to import before startup
 * `/blocks/` folder with numbered singleton blocks to import before startup
 * `/keys/` contains account keys that should be imported before startup

Client startup scripts need to ensure that they load the genesis state first, then import a possibly longer blockchain and then import possibly numerous individual blocks. The reason for requiring two different block sources is that specifying a single chain is more optimal, but tests requiring forking chains cannot create a single chain.

Besides the standardized chain configurations, clients can in general be modified behavior-wise in quite a few ways that are mostly supported by all clients, yet are implemented differently in each. As such, each possible behavioral change required by some simulator is characterized by an environment variable, which clients should interpret as best as they can.

The behavioral configuration variables:

  * `HIVE_BOOTNODE` enode URL of the discovery-only node to bootstrap the client
  * `HIVE_TESTNET` whether clients should run with modified starting nonces (`2^20`)
  * `HIVE_NODETYPE` specifying the sync and pruning algos that should be used
    * If unset, then uninteresting and run in the node's default mode
    * If `archive`, assumes that all historical state is retained after sync
    * If `full`, assumes fast sync and consecutive pruning of historical state
    * If `light`, assumes header only sync and no state maintenance at all
  * `HIVE_FORK_HOMESTEAD` the block number of the Ethereum Homestead transition
  * `HIVE_FORK_DAO_BLOCK` the block number of the DAO hard-fork transition
  * `HIVE_FORK_DAO_VOTE` whether the node supports or opposes the DAO hard-fork
  * `HIVE_FORK_TANGERINE` the block number of the Ethereum TangerineWhistle transition
    * The HF for repricing certain opcodes, EIP 150
  * `HIVE_FORK_SPURIOUS` the block number of the Ethereum Homestead transition
    * The HF for replay protection, state cleaning etc. EIPs 155,160,161. 
  * `HIVE_FORK_METROPOLIS` the block number of the Metropolis hardfork
  * `HIVE_MINER` address to credit with mining rewards (if set, start mining)
  * `HIVE_MINER_EXTRA` extra-data field to set for newly minted blocks

The client has the responsibility of mapping the hive environment variables to its own command line flags. To assist in this, Hive illustrates a technique
in the `clients/go-ethereum` folder using `mapper.jq`, which is invoked in `geth.sh` This technique can be replicated for other clients.

## Enode script

For devp2p tests or other simulations that require to know the specific enode URL of the client instance, the client must provide an `enode.sh` that echoes the enode of the running instance. This is executed by the Hive host remotely in order to retrieve the enode URL. 

## Starting the client

After initializing the client blockchain (genesis, chain, blocks), the last task of the entry script is to start up the client itself. The following defaults are required by `hive` to enable automatic network assembly and firewall enforcement:

 * Clients should open their HTTP-RPC endpoint on `0.0.0.0:8545` (mandatory)
 * Clients should open their WS-RPC endpoint on `0.0.0.0:8546` (optional)
 * Clients should open their IPC-RPC endpoints at `/rpc.ipc` (optional)

There is no need to handle graceful client termination. Clients will be forcefully aborted upon test suite completion and all related data purged. A new instance will be started for every test.

### Smoke testing new clients

To quickly check if a client adheres to the requirements of `hive`, there is a suite of smoke test simulations that just initialize clients with some pre-configured states and queries it from the various RPC endpoints.

```
$ hive --client=go-ethereum_latest --sim smoke
...
Simulation results:
{
  "go-ethereum:latest": {
    "smoke/lifecycle": {
      "start": "2017-01-31T09:20:16.975219924Z",
      "end": "2017-01-31T09:20:18.705302536Z",
      "success": true
    }
  }
}
```

*Note: All smoke tests must pass for a client to be included into `hive`.*

# Generating test blockchains with the `hivechain` tool

hive's `hivechain` tool allows you to create RLP-encoded blockchains for inclusion into simulations.

## Generating a blockchain

Build the `hivechain` tool, located in the `cmd/` directory.

Then, to generate a chain of a desired length, run the following command: 

```bash
hivechain generate -genesis <path to genesis file> -length <desired length of chain>
```
The `hivechain` tool will generate blocks with transactions as well if the following accounts are present and have a balance in the genesis block: 

```text
"0x71562b71999873DB5b286dF957af199Ec94617F7"
"0x703c4b2bD70c169f5717101CaeE543299Fc946C7"
"0x0D3ab14BBaD3D99F4203bd7a11aCB94882050E7e"
```

### Additional options: 

```text
  -blocktime int
    	The desired block time in seconds (default 30)
  -genesis string
    	The path and filename to the source genesis.json
  -length int
    	The length of the chain to generate (default 2)
  -mine
    	Enables ethash mining
  -output string
    	Chain destination folder (default ".")
  -tx-interval int
    	Add transaction to chain every n blocks (default 10)
```

# Trophies

If you find a bug in your client implementation due to this project, please be so
kind as to add it here to the trophy list. It could help prove that `hive` is indeed
a useful tool for validating Ethereum client implementations.

 * go-ethereum
   * Genesis chain config couldn't handle present but empty settings: [#2790](https://github.com/ethereum/go-ethereum/pull/2790)
   * Data race between remote block import and local block mining: [#2793](https://github.com/ethereum/go-ethereum/pull/2793)
   * Downloader didn't penalize incompatible forks hashly enough: [#2801](https://github.com/ethereum/go-ethereum/pull/2801)
 * Nethermind
   * Bug in p2p whith bonding nodes algorithm found by Hive: [#1894](https://github.com/NethermindEth/nethermind/pull/1894)

# Contributions

This project takes a different approach to code contributions than your usual FOSS project with well ingrained maintainers and relatively few external contributors. It is an experiment. Whether it will work out or not is for the future to decide.

We follow the [Collective Code Construction Contract (C4)](http://rfc.zeromq.org/spec:22/C4/), code
contribution model, as expanded and explained in [The ZeroMQ Process](https://hintjens.gitbooks.io/social-architecture/content/chapter4.html).
The core idea being that any patch that successfully solves an issue (bug/feature) and doesn't break
any existing code/contracts **must** be optimistically merged by maintainers. Followup patches may
be used to for additional polishes – and patches may even be outright reverted if they turn out to
have a negative impact – but no change must be rejected based on personal values.

Please consult the two C4 documents for details:

 * [Collective Code Construction Contract (C4)](http://rfc.zeromq.org/spec:22/C4/)
 * [The ZeroMQ Process](https://hintjens.gitbooks.io/social-architecture/content/chapter4.html)

# License

The `hive` project is licensed under the [GNU General Public License v3.0](http://www.gnu.org/licenses/gpl-3.0.en.html),
also included in our repository in the COPYING file.

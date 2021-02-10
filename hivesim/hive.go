package hivesim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/hive/internal/libhive"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Simulation wraps the simulation HTTP API provided by hive.
type Simulation struct {
	url string
}

// New looks up the hive host URI using the HIVE_SIMULATOR environment variable
// and connects to it. It will panic if HIVE_SIMULATOR is not set.
func New() *Simulation {
	simulator, isSet := os.LookupEnv("HIVE_SIMULATOR")
	if !isSet {
		panic("HIVE_SIMULATOR environment variable not set")
	}
	return &Simulation{url: simulator}
}

// NewAt creates a simulation connected to the given API endpoint. You'll will rarely need
// to use this. In simulations launched by hive, use New() instead.
func NewAt(url string) *Simulation {
	return &Simulation{url: url}
}

// EndTest finishes the test case, cleaning up everything, logging results, and returning
// an error if the process could not be completed.
func (sim *Simulation) EndTest(testSuite SuiteID, test TestID, summaryResult TestResult) error {
	// post results (which deletes the test case - because DELETE message body is not always supported)
	summaryResultData, err := json.Marshal(summaryResult)
	if err != nil {
		return err
	}

	vals := make(url.Values)
	vals.Add("summaryresult", string(summaryResultData))

	_, err = wrapHTTPErrorsPost(fmt.Sprintf("%s/testsuite/%d/test/%d", sim.url, testSuite, test), vals)
	return err
}

// StartSuite signals the start of a test suite.
func (sim *Simulation) StartSuite(name, description, simlog string) (SuiteID, error) {
	vals := make(url.Values)
	vals.Add("name", name)
	vals.Add("description", description)
	vals.Add("simlog", simlog)
	idstring, err := wrapHTTPErrorsPost(fmt.Sprintf("%s/testsuite", sim.url), vals)
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(idstring)
	if err != nil {
		return 0, err
	}
	return SuiteID(id), nil
}

// EndSuite signals the end of a test suite.
func (sim *Simulation) EndSuite(testSuite SuiteID) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/testsuite/%d", sim.url, testSuite), nil)
	if err != nil {
		return err
	}
	_, err = http.DefaultClient.Do(req)
	return err
}

// StartTest starts a new test case, returning the testcase id as a context identifier.
func (sim *Simulation) StartTest(testSuite SuiteID, name string, description string) (TestID, error) {
	vals := make(url.Values)
	vals.Add("name", name)
	vals.Add("description", description)

	idstring, err := wrapHTTPErrorsPost(fmt.Sprintf("%s/testsuite/%d/test", sim.url, testSuite), vals)
	if err != nil {
		return 0, err
	}
	testID, err := strconv.Atoi(idstring)
	if err != nil {
		return 0, err
	}
	return TestID(testID), nil
}

// ClientTypes returns all client types available to this simulator run. This depends on
// both the available client set and the command line filters.
func (sim *Simulation) ClientTypes() (availableClients []*libhive.ClientDefinition, err error) {
	resp, err := http.Get(fmt.Sprintf("%s/clients", sim.url))
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &availableClients)
	if err != nil {
		return nil, err
	}
	return
}

// StartClient starts a new node (or other container) with the specified parameters. One
// parameter must be named CLIENT and should contain one of the client types from
// GetClientTypes. The input is used as environment variables in the new container.
// Returns container id and ip.
func (sim *Simulation) StartClient(testSuite SuiteID, test TestID, parameters map[string]string, initFiles map[string]string) (string, net.IP, error) {
	clientType, ok := parameters["CLIENT"]
	if !ok {
		return "", nil, errors.New("missing 'CLIENT' parameter")
	}
	return sim.StartClientWithOptions(testSuite, test, clientType, WithParams(parameters), WithFiles(initFiles))
}

// StartClientWithOptions starts a new node (or other container) with specified options.
// Returns container id and ip.
func (sim *Simulation) StartClientWithOptions(testSuite SuiteID, test TestID, clientType string, options ...StartOption) (string, net.IP, error) {
	setup := &clientSetup{
		parameters: make(map[string]string),
		initFiles:  make(map[string]string),
	}
	setup.parameters["CLIENT"] = clientType
	for _, opt := range options {
		opt(setup)
	}
	data, err := setup.postWithFiles(fmt.Sprintf("%s/testsuite/%d/test/%d/node", sim.url, testSuite, test))
	if err != nil {
		return "", nil, err
	}
	if idip := strings.Split(data, "@"); len(idip) >= 1 {
		return idip[0], net.ParseIP(idip[1]), nil
	}
	return data, net.IP{}, fmt.Errorf("no ip address returned: %v", data)
}

// StopClient signals to the host that the node is no longer required.
func (sim *Simulation) StopClient(testSuite SuiteID, test TestID, nodeid string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/testsuite/%d/test/%d/node/%s", sim.url, testSuite, test, nodeid), nil)
	if err != nil {
		return err
	}
	_, err = http.DefaultClient.Do(req)
	return err
}

// ClientEnodeURL returns the enode URL of a running client.
func (sim *Simulation) ClientEnodeURL(testSuite SuiteID, test TestID, node string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/testsuite/%d/test/%d/node/%s", sim.url, testSuite, test, node))
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	res := strings.TrimRight(string(body), "\r\n")
	return res, nil
}

type execInfo struct {
	StdOut   string `json:"out"`
	StdErr   string `json:"err"`
	ExitCode int    `json:"code"`
}

// ClientRunProgram runs a command in a running client.
func (sim *Simulation) ClientRunProgram(testSuite SuiteID, test TestID,
	nodeid string, privileged bool, user string, cmd string) (stdOut string, stdErr string, exitCode int, err error) {

	params := url.Values{}
	params.Add("privileged", strconv.FormatBool(privileged))
	params.Add("user", user)
	params.Add("cmd", cmd)
	p := fmt.Sprintf("%s/testsuite/%d/test/%d/node/%s/exec?%s", sim.url, testSuite, test, nodeid, params.Encode())
	req, err := http.NewRequest(http.MethodPost, p, nil)
	if err != nil {
		return "", "", 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	if resp.Body == nil {
		return "", "", 0, errors.New("unexpected empty response body")
	}
	dec := json.NewDecoder(resp.Body)
	var res execInfo
	if err := dec.Decode(&res); err != nil {
		return "", "", 0, err
	}
	return res.StdOut, res.StdErr, res.ExitCode, err
}

// CreateNetwork sends a request to the hive server to create a docker network by
// the given name.
func (sim *Simulation) CreateNetwork(testSuite SuiteID, networkName string) error {
	_, err := http.Post(fmt.Sprintf("%s/testsuite/%d/network/%s", sim.url, testSuite, networkName), "application/json", nil)
	return err
}

// RemoveNetwork sends a request to the hive server to remove the given network.
func (sim *Simulation) RemoveNetwork(testSuite SuiteID, network string) error {
	endpoint := fmt.Sprintf("%s/testsuite/%d/network/%s", sim.url, testSuite, network)
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	_, err = http.DefaultClient.Do(req)
	return err
}

// ConnectContainer sends a request to the hive server to connect the given
// container to the given network.
func (sim *Simulation) ConnectContainer(testSuite SuiteID, network, containerID string) error {
	endpoint := fmt.Sprintf("%s/testsuite/%d/network/%s/%s", sim.url, testSuite, network, containerID)
	_, err := http.Post(endpoint, "application/json", nil)
	return err
}

// DisconnectContainer sends a request to the hive server to disconnect the given
// container from the given network.
func (sim *Simulation) DisconnectContainer(testSuite SuiteID, network, containerID string) error {
	endpoint := fmt.Sprintf("%s/testsuite/%d/network/%s/%s", sim.url, testSuite, network, containerID)
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	_, err = http.DefaultClient.Do(req)
	return err
}

// ContainerNetworkIP returns the IP address of a container on the given network. If the
// container ID is "simulation", it returns the IP address of the simulator container.
func (sim *Simulation) ContainerNetworkIP(testSuite SuiteID, network, containerID string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/testsuite/%d/network/%s/%s", sim.url, testSuite, network, containerID))
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// collects client setup options
type clientSetup struct {
	parameters map[string]string
	initFiles  map[string]string
	tars       []func() io.ReadCloser
}

type StartOption func(setup *clientSetup)

// WithParams adds parameters to the client setup, which are put into the docker env.
func WithParams(params Params) StartOption {
	return func(setup *clientSetup) {
		for k, v := range params {
			setup.parameters[k] = v
		}
	}
}

// WithFiles adds files from the local filesystem to the client.
func WithFiles(initFiles map[string]string) StartOption {
	return func(setup *clientSetup) {
		for k, v := range initFiles {
			setup.initFiles[k] = v
		}
	}
}

// WithTAR adds a TAR archive as source for client files.
func WithTAR(src func() io.ReadCloser) StartOption {
	return func(setup *clientSetup) {
		setup.tars = append(setup.tars, src)
	}
}

func (setup *clientSetup) postWithFiles(url string) (string, error) {
	var err error

	// make a dictionary of readers
	formValues := make(map[string]io.Reader)
	for key, s := range setup.parameters {
		formValues[key] = strings.NewReader(s)
	}
	for key, filename := range setup.initFiles {
		filereader, err := os.Open(filename)
		if err != nil {
			return "", err
		}
		//fi, err := filereader.Stat()
		formValues[key] = filereader
	}

	// send them
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, r := range formValues {
		var fw io.Writer
		if x, ok := r.(io.Closer); ok {
			defer x.Close()
		}
		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return "", err
			}
		} else {
			if fw, err = w.CreateFormField(key); err != nil {
				return "", err
			}
		}
		if _, err = io.Copy(fw, r); err != nil {
			return "", err
		}
	}

	for i, src := range setup.tars {
		h := make(textproto.MIMEHeader)
		filename := fmt.Sprintf("hive_tar_%d", i)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, filename, filename))
		h.Set("Content-Type", "application/octet-stream")
		h.Set("X-HIVE-FILETYPE", "TAR")
		fw, err := w.CreatePart(h)
		if err != nil {
			return "", err
		}
		r := src()
		_, err = io.Copy(fw, r)
		r.Close()
		if err != nil {
			return "", err
		}
	}

	// this must be closed or the request will be missing the terminating boundary
	w.Close()

	// Can't use http.PostForm because we need to change the content header
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return "", err
	}
	// Set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Submit the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		return string(body), nil
	}
	return "", fmt.Errorf("request failed (%d): %v", resp.StatusCode, string(body))
}

// wrapHttpErrorsPost wraps http.PostForm to convert responses that are not 200 OK into errors
func wrapHTTPErrorsPost(url string, data url.Values) (string, error) {
	resp, err := http.PostForm(url, data)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 300 {
		return string(body), nil
	}
	return "", fmt.Errorf("request failed (%d): %v", resp.StatusCode, string(body))
}

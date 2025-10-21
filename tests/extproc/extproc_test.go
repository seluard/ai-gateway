// Copyright Envoy AI Gateway Authors
// SPDX-License-Identifier: Apache-2.0
// The full text of the Apache license is available in the LICENSE file at
// the root of the repo.

package extproc

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/envoyproxy/ai-gateway/internal/filterapi"
	"github.com/envoyproxy/ai-gateway/tests/internal/testenvironment"
)

const (
	// errorServerDefaultPort is a port we need to replace in envoyConfig.
	errorServerDefaultPort = 1066
	// errorServerTLSDefaultPort is a port we need to replace in envoyConfig.
	errorServerTLSDefaultPort = 1067
	eventuallyTimeout         = 20 * time.Second
	eventuallyInterval        = 10 * time.Millisecond
	fakeGCPAuthToken          = "fake-gcp-auth-token" //nolint:gosec
)

var (
	openAISchema         = filterapi.VersionedAPISchema{Name: filterapi.APISchemaOpenAI, Version: "v1"}
	awsBedrockSchema     = filterapi.VersionedAPISchema{Name: filterapi.APISchemaAWSBedrock}
	awsAnthropicSchema   = filterapi.VersionedAPISchema{Name: filterapi.APISchemaAWSAnthropic}
	azureOpenAISchema    = filterapi.VersionedAPISchema{Name: filterapi.APISchemaAzureOpenAI, Version: "2025-01-01-preview"}
	gcpVertexAISchema    = filterapi.VersionedAPISchema{Name: filterapi.APISchemaGCPVertexAI}
	gcpAnthropicAISchema = filterapi.VersionedAPISchema{Name: filterapi.APISchemaGCPAnthropic, Version: "vertex-2023-10-16"}
	geminiSchema         = filterapi.VersionedAPISchema{Name: filterapi.APISchemaOpenAI, Version: "v1beta/openai"}
	groqSchema           = filterapi.VersionedAPISchema{Name: filterapi.APISchemaOpenAI, Version: "openai/v1"}
	grokSchema           = filterapi.VersionedAPISchema{Name: filterapi.APISchemaOpenAI, Version: "v1"}
	sambaNovaSchema      = filterapi.VersionedAPISchema{Name: filterapi.APISchemaOpenAI, Version: "v1"}
	deepInfraSchema      = filterapi.VersionedAPISchema{Name: filterapi.APISchemaOpenAI, Version: "v1/openai"}

	testUpstreamOpenAIBackend      = filterapi.Backend{Name: "testupstream-openai", Schema: openAISchema}
	testUpstreamModelNameOverride  = filterapi.Backend{Name: "testupstream-modelname-override", ModelNameOverride: "override-model", Schema: openAISchema}
	testUpstreamAAWSBackend        = filterapi.Backend{Name: "testupstream-aws", Schema: awsBedrockSchema}
	testUpstreamAzureBackend       = filterapi.Backend{Name: "testupstream-azure", Schema: azureOpenAISchema}
	testUpstreamGCPVertexAIBackend = filterapi.Backend{Name: "testupstream-gcp-vertexai", Schema: gcpVertexAISchema, Auth: &filterapi.BackendAuth{GCPAuth: &filterapi.GCPAuth{
		AccessToken: fakeGCPAuthToken,
		Region:      "gcp-region",
		ProjectName: "gcp-project-name",
	}}}
	testUpstreamGCPAnthropicAIBackend = filterapi.Backend{Name: "testupstream-gcp-anthropicai", Schema: gcpAnthropicAISchema, Auth: &filterapi.BackendAuth{GCPAuth: &filterapi.GCPAuth{
		AccessToken: fakeGCPAuthToken,
		Region:      "gcp-region",
		ProjectName: "gcp-project-name",
	}}}
	// This always failing backend is configured to have AWS Bedrock schema so that
	// we can test that the extproc can fallback to the different schema. E.g. Primary AWS and then OpenAI.
	alwaysFailingBackend = filterapi.Backend{Name: "always-failing-backend", Schema: awsBedrockSchema}

	// envoyConfig is the embedded Envoy configuration template.
	//
	//go:embed envoy.yaml
	envoyConfig string

	// extprocBin holds the path to the compiled extproc binary.
	extprocBin string

	// testupstreamBin holds the path to the compiled testupstream binary.
	testupstreamBin string
)

// TestMain sets up the test environment once for all tests.
func TestMain(m *testing.M) {
	var err error
	// Build extproc binary once for all tests.
	if extprocBin, err = BuildExtProcOnDemand(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start tests due to extproc build error: %v\n", err)
		os.Exit(1)
	}

	// Build testupstream binary once for all tests.
	if testupstreamBin, err = buildTestUpstreamOnDemand(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start tests due to testupstream build error: %v\n", err)
		os.Exit(1)
	}

	// This is a fake server that returns a 500 error for all requests.
	errorServerMux := http.NewServeMux()
	errorServerMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	})

	ctx := context.Background()
	errorServerLis, errorServerPort := listen(ctx, "error server")
	errorServerTLSLis, errorServerTLSPort := listen(ctx, "error TLS server")

	envoyConfig = strings.ReplaceAll(
		envoyConfig,
		"port_value: "+strconv.Itoa(errorServerDefaultPort),
		"port_value: "+strconv.Itoa(errorServerPort),
	)

	envoyConfig = strings.ReplaceAll(
		envoyConfig,
		"port_value: "+strconv.Itoa(errorServerTLSDefaultPort),
		"port_value: "+strconv.Itoa(errorServerTLSPort),
	)

	errorServer := &http.Server{Handler: errorServerMux, ReadHeaderTimeout: 5 * time.Second}
	errorServerTLS := &http.Server{Handler: errorServerMux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := errorServer.Serve(errorServerLis); err != nil && !strings.Contains(err.Error(), "Server closed") {
			panic(fmt.Sprintf("error starting HTTP server: %v", err))
		}
	}()
	go func() {
		if err := errorServerTLS.ServeTLS(errorServerTLSLis, "testdata/server.crt", "testdata/server.key"); err != nil &&
			!strings.Contains(err.Error(), "Server closed") {
			panic(fmt.Sprintf("error starting HTTPS server: %v", err))
		}
	}()

	// Run tests.
	res := m.Run()
	_ = errorServer.Close()
	_ = errorServerTLS.Close()
	os.Exit(res)
}

func startTestEnvironment(t testing.TB, extprocConfig string, okToDumpLogOnFailure, extProcInProcess bool) *testenvironment.TestEnvironment {
	return testenvironment.StartTestEnvironment(t,
		requireUpstream, map[string]int{"upstream": 8080},
		extprocBin, extprocConfig, nil, envoyConfig, okToDumpLogOnFailure, extProcInProcess, 120*time.Second,
	)
}

// requireUpstream starts the external processor with the given configuration.
func requireUpstream(t testing.TB, out io.Writer, ports map[string]int) {
	cmd := exec.CommandContext(t.Context(), testupstreamBin)
	cmd.Env = append(os.Environ(),
		"TESTUPSTREAM_ID=extproc_test",
		fmt.Sprintf("LISTENER_PORT=%d", ports["upstream"]))

	// wait for the ready message or exit.
	testenvironment.StartAndAwaitReady(t, cmd, out, out, "Test upstream is ready")
}

func listen(ctx context.Context, name string) (net.Listener, int) {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Errorf("failed to listen for %s: %w", name, err))
	}
	return lis, lis.Addr().(*net.TCPAddr).Port
}

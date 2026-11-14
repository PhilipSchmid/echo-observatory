package integration_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevWatcherIncludesEmbeddedAssets(t *testing.T) {
	cfg := viper.New()
	cfg.SetConfigFile("../.air.toml")
	require.NoError(t, cfg.ReadInConfig())
	assert.Equal(t, "make build-quick APP_NAME=./tmp/air/echo-observatory", cfg.GetString("build.cmd"))
	assert.Equal(t, []string{"./tmp/air/echo-observatory"}, cfg.GetStringSlice("build.entrypoint"))
	for _, extension := range []string{"go", "html", "css", "js", "svg", "png", "woff2"} {
		assert.Contains(t, cfg.GetStringSlice("build.include_ext"), extension)
	}
	assert.Contains(t, cfg.GetStringSlice("build.exclude_dir"), "tmp")
	assert.NotContains(t, cfg.GetStringSlice("build.exclude_dir"), "assets")
	assert.True(t, cfg.GetBool("build.send_interrupt"))
	assert.True(t, cfg.GetBool("build.stop_on_error"))
	assert.False(t, cfg.GetBool("proxy.enabled"), "keep direct diagnostics and CSP")
	assert.Contains(t, cfg.GetStringSlice("build.args_bin"), "--tls-port=18843")
	assert.Contains(t, cfg.GetStringSlice("build.args_bin"), "--quic-port=18843")
}

func TestDeploymentPortsStayConsistent(t *testing.T) {
	expected := map[string]int{"tcp/80": 8080, "tcp/443": 8443, "udp/443": 4433, "tcp/50051": 50051, "tcp/9090": 9090}
	assertAdvertisedPorts := func(t *testing.T, environment map[string]string) {
		t.Helper()
		for protocol, port := range map[string]string{"http": "80", "tls": "443", "tcp": "9090", "grpc": "50051", "quic": "443"} {
			assert.Equal(t, port, environment["echo_observatory_"+protocol+"_advertise_port"], "Connect must advertise the exposed %s port", protocol)
		}
	}
	t.Run("Compose", func(t *testing.T) {
		cfg := viper.New()
		cfg.SetConfigFile("../compose.yaml")
		require.NoError(t, cfg.ReadInConfig())
		assert.True(t, cfg.GetBool("networks.default.enable_ipv6"))
		var compose struct {
			Services map[string]struct {
				Ports       []string
				Environment map[string]string
				ReadOnly    bool     `mapstructure:"read_only"`
				CapDrop     []string `mapstructure:"cap_drop"`
			}
		}
		require.NoError(t, cfg.Unmarshal(&compose))
		service, ok := compose.Services["echo-observatory"]
		require.True(t, ok)
		assert.ElementsMatch(t, []string{"80:8080/tcp", "443:8443/tcp", "443:4433/udp", "9090:9090/tcp", "50051:50051/tcp", "127.0.0.1:3000:3000/tcp", "[::1]:3000:3000/tcp"}, service.Ports)
		assertAdvertisedPorts(t, service.Environment)
		assert.Equal(t, "false", service.Environment["echo_observatory_protection"], "self-hosted examples must not inherit public-demo quotas")
		assert.Equal(t, "true", service.Environment["echo_observatory_grpc_tls"])
		assert.True(t, service.ReadOnly)
		assert.Equal(t, []string{"ALL"}, service.CapDrop)
	})
	t.Run("Kubernetes documentation", func(t *testing.T) {
		data, err := os.ReadFile("../docs/kubernetes.md")
		require.NoError(t, err)
		blocks := strings.Split(string(data), "```yaml\n")
		require.Len(t, blocks, 2)
		manifest := strings.SplitN(blocks[1], "```", 2)[0]
		found := make(map[string]map[string]int)
		for _, document := range strings.Split(manifest, "---\n") {
			if strings.TrimSpace(document) == "" {
				continue
			}
			cfg := viper.New()
			cfg.SetConfigType("yaml")
			require.NoError(t, cfg.ReadConfig(strings.NewReader(document)))
			if cfg.GetString("kind") == "ConfigMap" {
				assertAdvertisedPorts(t, cfg.GetStringMapString("data"))
				assert.Equal(t, "false", cfg.GetStringMapString("data")["echo_observatory_protection"], "keep self-hosted defaults with the pinned prerelease too")
			}
			if cfg.GetString("kind") != "Service" {
				continue
			}
			var service struct {
				Spec struct {
					Type  string
					Ports []struct {
						Protocol   string
						Port       int
						TargetPort int `mapstructure:"targetPort"`
					}
				}
			}
			require.NoError(t, cfg.Unmarshal(&service))
			assert.Equal(t, "ClusterIP", service.Spec.Type)
			assert.Equal(t, "PreferDualStack", cfg.GetString("spec.ipFamilyPolicy"))
			ports := make(map[string]int)
			for _, port := range service.Spec.Ports {
				protocol := strings.ToLower(port.Protocol)
				if protocol == "" {
					protocol = "tcp"
				}
				key := protocol + "/" + strconv.Itoa(port.Port)
				_, duplicate := ports[key]
				assert.False(t, duplicate, key)
				ports[key] = port.TargetPort
			}
			found[cfg.GetString("metadata.name")] = ports
		}
		assert.Equal(t, expected, found["echo-observatory-service"])
		assert.Equal(t, map[string]int{"tcp/3000": 3000}, found["echo-observatory-admin"])
		require.Len(t, found, 2)
	})
}

func TestReleaseWaitsForImagesAndProtectsStableTags(t *testing.T) {
	cfg := viper.New()
	cfg.SetConfigFile("../.github/workflows/release.yaml")
	require.NoError(t, cfg.ReadInConfig())
	assert.Equal(t, "checks", cfg.GetString("jobs.image-smoke.needs"))
	assert.Equal(t, "image-smoke", cfg.GetString("jobs.docker-release.needs"))
	assert.Equal(t, "docker-release", cfg.GetString("jobs.release.needs"), "publish the release only after its images and SBOM exist")
	assert.ElementsMatch(t, []string{"docker-release", "release"}, cfg.GetStringSlice("jobs.deploy.needs"), "deploy only after publication succeeds")
	assert.Contains(t, cfg.GetString("jobs.deploy.if"), "github.repository == 'PhilipSchmid/echo-observatory'", "forks must not deploy the public demo")
	assert.Contains(t, cfg.GetString("env.image_name"), "'philipschmid/echo-app' || 'philipschmid/echo-observatory'")
	var steps []struct {
		Uses string
		With map[string]any
	}
	require.NoError(t, cfg.UnmarshalKey("jobs.docker-release.steps", &steps))
	var flavor string
	var imagePublished, sbomBeforeRelease bool
	for _, step := range steps {
		if strings.HasPrefix(step.Uses, "docker/build-push-action@") {
			imagePublished = true
			assert.Equal(t, true, step.With["push"])
			assert.Equal(t, true, step.With["sbom"])
			assert.Equal(t, "mode=max", step.With["provenance"])
			assert.Equal(t, "${{ steps.meta.outputs.tags }}", step.With["tags"])
		}
		if strings.HasPrefix(step.Uses, "anchore/sbom-action@") {
			assert.Equal(t, false, step.With["upload-release-assets"])
			sbomBeforeRelease = true
		}
		tags, _ := step.With["tags"].(string)
		if strings.Contains(tags, "type=semver") {
			flavor, _ = step.With["flavor"].(string)
			assert.NotContains(t, tags, "type=raw,value=latest")
		}
	}
	assert.True(t, imagePublished)
	assert.True(t, sbomBeforeRelease)
	assert.Equal(t, "latest=${{ !contains(github.ref_name, '-') }}", flavor)
}

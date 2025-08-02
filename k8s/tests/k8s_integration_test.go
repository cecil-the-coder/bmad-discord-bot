package k8s_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testNamespace = "bmad-bot-test"
	timeout       = 30 * time.Second
)

// TestKubernetesManifests tests the Kubernetes manifests validation
func TestKubernetesManifests(t *testing.T) {
	// Skip integration tests if running in CI without cluster
	if os.Getenv("SKIP_K8S_INTEGRATION") == "true" {
		t.Skip("Skipping Kubernetes integration tests")
	}

	client := getKubernetesClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	t.Run("Namespace", func(t *testing.T) {
		testNamespaceManifest(t, ctx, client)
	})

	t.Run("ServiceAccount", func(t *testing.T) {
		testServiceAccountManifest(t, ctx, client)
	})

	t.Run("ConfigMap", func(t *testing.T) {
		testConfigMapManifest(t, ctx, client)
	})

	t.Run("Deployment", func(t *testing.T) {
		testDeploymentManifest(t, ctx, client)
	})

}

func getKubernetesClient(t *testing.T) kubernetes.Interface {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		homeDir, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(homeDir, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Skipf("Cannot load kubeconfig: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err, "Failed to create Kubernetes client")

	return client
}

func testNamespaceManifest(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	// Load namespace manifest
	manifest := loadManifest(t, "../namespace.yaml")
	var namespace corev1.Namespace
	err := yaml.Unmarshal(manifest, &namespace)
	require.NoError(t, err, "Failed to unmarshal namespace manifest")

	// Validate namespace properties
	assert.Equal(t, "bmad-bot", namespace.Name)
	assert.Equal(t, "bmad-bot", namespace.Labels["name"])
	assert.Equal(t, "bmad-discord-bot", namespace.Labels["app"])
	assert.Equal(t, "production", namespace.Labels["environment"])
}

func testServiceAccountManifest(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	// Load serviceaccount manifest
	manifest := loadManifest(t, "../serviceaccount.yaml")

	// Split the manifest by "---" to handle multiple resources
	manifests := splitYAMLManifests(manifest)
	require.GreaterOrEqual(t, len(manifests), 1, "Expected at least 1 resource in serviceaccount manifest")

	var serviceAccount corev1.ServiceAccount
	err := yaml.Unmarshal(manifests[0], &serviceAccount)
	require.NoError(t, err, "Failed to unmarshal serviceaccount manifest")

	// Validate serviceaccount properties
	assert.Equal(t, "bmad-bot-sa", serviceAccount.Name)
	assert.Equal(t, "bmad-bot", serviceAccount.Namespace)
	assert.False(t, *serviceAccount.AutomountServiceAccountToken)
}

func testConfigMapManifest(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	// Load configmap manifest
	manifest := loadManifest(t, "../configmap.yaml")
	var configMap corev1.ConfigMap
	err := yaml.Unmarshal(manifest, &configMap)
	require.NoError(t, err, "Failed to unmarshal configmap manifest")

	// Validate configmap properties
	assert.Equal(t, "bmad-bot-config", configMap.Name)
	assert.Equal(t, "bmad-bot", configMap.Namespace)

	// Validate required configuration keys
	requiredKeys := []string{
		"DATABASE_TYPE",
		"MYSQL_HOST",
		"MYSQL_PORT",
		"AI_PROVIDER",
		"OLLAMA_HOST",
		"OLLAMA_MODEL",
	}

	for _, key := range requiredKeys {
		assert.Contains(t, configMap.Data, key, "ConfigMap should contain %s", key)
	}

	// Validate specific values
	assert.Equal(t, "mysql", configMap.Data["DATABASE_TYPE"])
	assert.Equal(t, "ollama", configMap.Data["AI_PROVIDER"])
}

func testDeploymentManifest(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	// Load deployment manifest
	manifest := loadManifest(t, "../deployment.yaml")
	var deployment appsv1.Deployment
	err := yaml.Unmarshal(manifest, &deployment)
	require.NoError(t, err, "Failed to unmarshal deployment manifest")

	// Validate deployment properties
	assert.Equal(t, "bmad-discord-bot", deployment.Name)
	assert.Equal(t, "bmad-bot", deployment.Namespace)
	assert.Equal(t, int32(1), *deployment.Spec.Replicas)

	// Validate container specification
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	container := deployment.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "bmad-discord-bot", container.Name)
	assert.Contains(t, container.Image, "bmad-discord-bot")

	// Validate resource limits
	limits := container.Resources.Limits
	assert.Equal(t, "512Mi", limits.Memory().String())
	assert.Equal(t, "500m", limits.Cpu().String())

	// Validate resource requests
	requests := container.Resources.Requests
	assert.Equal(t, "256Mi", requests.Memory().String())
	assert.Equal(t, "250m", requests.Cpu().String())

	// Validate security context
	securityContext := container.SecurityContext
	require.NotNil(t, securityContext)
	assert.False(t, *securityContext.AllowPrivilegeEscalation)
	assert.True(t, *securityContext.ReadOnlyRootFilesystem)
	assert.True(t, *securityContext.RunAsNonRoot)
	assert.Equal(t, int64(1000), *securityContext.RunAsUser)

	// Validate health checks
	assert.NotNil(t, container.LivenessProbe)
	assert.NotNil(t, container.ReadinessProbe)
	assert.NotNil(t, container.StartupProbe)

	// Validate environment configuration
	var hasConfigMapRef, hasSecretRef bool
	for _, envFrom := range container.EnvFrom {
		if envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name == "bmad-bot-config" {
			hasConfigMapRef = true
		}
		if envFrom.SecretRef != nil && envFrom.SecretRef.Name == "bmad-bot-secrets" {
			hasSecretRef = true
		}
	}
	assert.True(t, hasConfigMapRef, "Container should reference bmad-bot-config ConfigMap")
	assert.True(t, hasSecretRef, "Container should reference bmad-bot-secrets Secret")

	// Validate volume mounts
	expectedMounts := []string{"data-volume", "logs-volume", "temp-volume"}
	assert.Len(t, container.VolumeMounts, len(expectedMounts))

	for _, expectedMount := range expectedMounts {
		found := false
		for _, mount := range container.VolumeMounts {
			if mount.Name == expectedMount {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected volume mount %s not found", expectedMount)
	}
}

// TestKustomizeConfiguration tests the kustomization.yaml file
func TestKustomizeConfiguration(t *testing.T) {
	// Load kustomization manifest
	manifest := loadManifest(t, "../kustomization.yaml")

	// Basic validation that the file is valid YAML
	var kustomization map[string]interface{}
	err := yaml.Unmarshal(manifest, &kustomization)
	require.NoError(t, err, "Failed to unmarshal kustomization manifest")

	// Validate basic structure
	assert.Equal(t, "kustomize.config.k8s.io/v1beta1", kustomization["apiVersion"])
	assert.Equal(t, "Kustomization", kustomization["kind"])

	// Validate namespace
	assert.Equal(t, "bmad-bot", kustomization["namespace"])

	// Validate resources list exists
	resources, ok := kustomization["resources"].([]interface{})
	require.True(t, ok, "resources should be a list")

	expectedResources := []string{
		"namespace.yaml",
		"serviceaccount.yaml",
		"configmap.yaml",
		"secret.yaml",
		"deployment.yaml",
		"networkpolicy.yaml",
		"hpa.yaml",
	}

	assert.Len(t, resources, len(expectedResources))
}

// Helper functions

func loadManifest(t *testing.T, relativePath string) []byte {
	manifestPath := filepath.Join(getProjectRoot(), "k8s", relativePath)
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err, "Failed to read manifest file: %s", manifestPath)
	return manifest
}

func getProjectRoot() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "..", "..")
}

func splitYAMLManifests(data []byte) [][]byte {
	// Simple YAML document separator splitting
	// This is a basic implementation for test purposes
	parts := [][]byte{}
	current := []byte{}

	lines := string(data)
	for _, line := range []string{lines} {
		if line == "---" && len(current) > 0 {
			parts = append(parts, current)
			current = []byte{}
		} else {
			current = append(current, []byte(line)...)
		}
	}

	if len(current) > 0 {
		parts = append(parts, current)
	}

	if len(parts) == 0 {
		parts = append(parts, data)
	}

	return parts
}

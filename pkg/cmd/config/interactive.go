// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// InteractiveSetup handles the interactive configuration setup process
type InteractiveSetup struct {
	configManager ConfigManager
	discoverer    *discoverer
	namespace     string
}

// NewInteractiveSetup creates a new interactive setup instance
func NewInteractiveSetup(configManager ConfigManager, namespace string, tester ConnectivityTester) *InteractiveSetup {
	// Attempt automatic discovery with interactive TLS configuration
	k8sClient, errK8sClient := kubernetes.NewForConfig(configManager.GetK8sRESTConfig())
	if errK8sClient != nil {
		return nil
	}
	dynamicClient, errDynamicClient := dynamic.NewForConfig(configManager.GetK8sRESTConfig())
	if errDynamicClient != nil {
		return nil
	}
	configDiscoverer := discoverer{k8sClient, dynamicClient, tester}

	return &InteractiveSetup{
		configManager: configManager,
		discoverer:    &configDiscoverer,
		namespace:     namespace,
	}
}

// RunSetup executes the interactive setup process with additional context
func (is *InteractiveSetup) RunSetup() error {
	if overwrittenCluster, _ := is.configManager.GetCurrentClusterConfig(); overwrittenCluster != nil {
		fmt.Printf("Cluster '%s' is already configured.\n", overwrittenCluster.ClusterName)
		overwrittenCluster.DisplaySummary()
		fmt.Println()
		confirmed, err := PromptForConfirmation(
			"Do you want to reconfigure this cluster for KubeArchive CLI?",
			DefaultNo,
		)
		if err != nil {
			return fmt.Errorf("failed to confirmation: %w", err)
		}
		if !confirmed {
			return fmt.Errorf("setup cancelled")
		}
		err = is.configManager.RemoveClusterConfigByName(overwrittenCluster.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to remove cluster '%s': %w", overwrittenCluster.ClusterName, err)
		}
	}
	defaultClusterName, err := is.configManager.GenerateClusterName()
	if err != nil {
		return fmt.Errorf("failed to determine cluster name: %w", err)
	}

	confirmed, err := PromptForConfirmation(fmt.Sprintf("Do you want to configure KubeArchive CLI for the current cluster '%s'?", defaultClusterName), DefaultYes)
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	if !confirmed {
		return fmt.Errorf("setup cancelled")
	}

	fmt.Println()

	clusterNameResponse, err := is.PromptForInput(fmt.Sprintf("Cluster name [%s]", defaultClusterName))
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	clusterName := clusterNameResponse
	if clusterName == "" {
		clusterName = defaultClusterName
	}

	fmt.Println()
	fmt.Println("🔍 Discovering KubeArchive services in the cluster...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	host, err := is.discoverer.getKubeArchiveHost(ctx)
	if err != nil {
		fmt.Printf("⚠️  Automatic discovery failed: %v\n", err)
		fmt.Println()
		fmt.Println("❌ KubeArchive couldn't be automatically discovered.")
		fmt.Println()
		fmt.Println("This could happen if:")
		fmt.Println("  • KubeArchive is not installed in this cluster")
		fmt.Println("  • The service is not exposed externally")
		fmt.Println("  • Network connectivity issues")
		fmt.Println()
		fmt.Println("📞 Please contact the cluster admin for assistance.")
		fmt.Println("   Once KubeArchive is properly configured, you can set it up using:")
		fmt.Println()
		fmt.Printf("   kubectl ka config set host <host-url>\n")
		fmt.Println()
		fmt.Println("   Example:")
		fmt.Println("   kubectl ka config set host https://kubearchive-api.example.com")
		fmt.Println()
		return fmt.Errorf("KubeArchive couldn't be automatically discovered")
	}

	tlsInsecure, certPath, err := is.getTLSSetup(host)
	if err != nil {
		return fmt.Errorf("failed to automatic discover TLS Configuration for host %s: %w", host, err)
	}

	if certPath != "" {
		certData, errReadCert := os.ReadFile(certPath)
		if errReadCert != nil {
			return fmt.Errorf("failed to read certificate file: %w", errReadCert)
		}

		if is.discoverer.connectivityTester.TestKubeArchiveLivezEndpoint(host, tlsInsecure, certData) != nil {
			fmt.Printf("❌ Connection failed even with certificate: %s\n", certPath)
			return fmt.Errorf("endpoint not accessible with certificate: %s", host)
		}
	} else if tlsInsecure {
		if err = is.discoverer.connectivityTester.TestKubeArchiveLivezEndpoint(host, tlsInsecure, nil); err != nil {
			return fmt.Errorf("endpoint %s not accessible without TLS: %w", host, err)
		}
	}

	fmt.Println()
	fmt.Println("🔐 Setting up authentication...")
	token, err := is.getTokenForKubeArchive(host)

	if err != nil {
		return fmt.Errorf("authentication setup failed: %w", err)
	}

	clusterConfig := &ClusterConfig{
		ClusterName: clusterName,
		Host:        host,
		ServerURL:   is.configManager.GetK8sRESTConfig().Host,
		TLSInsecure: tlsInsecure,
		CertPath:    certPath,
		Token:       token,
	}

	clusterConfig.DisplaySummary()

	confirmed, err = PromptForConfirmation("Do you want to save this configuration?", DefaultYes)
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	if !confirmed {
		return fmt.Errorf("setup cancelled")
	}

	// Save config
	is.configManager.AddCluster(clusterConfig)
	err = is.configManager.SaveConfig()
	if err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf("Configuration saved successfully\n")

	return nil
}

// PromptForInput prompts the user for text input
func (is *InteractiveSetup) PromptForInput(message string) (string, error) {
	fmt.Print(message + ": ")
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		response = ""
	}
	return strings.TrimSpace(response), nil
}

// ConfirmationDefault represents the default behavior for confirmation prompts
type ConfirmationDefault int

const (
	DefaultNo ConfirmationDefault = iota
	DefaultYes
)

// Suffix returns the appropriate prompt suffix for the confirmation type
func (cd ConfirmationDefault) Suffix() string {
	switch cd {
	case DefaultYes:
		return " (Y/n): "
	case DefaultNo:
		return " (y/N): "
	default:
		return " (y/N): " // fallback to safer default
	}
}

// DefaultValue returns the boolean value to use when user provides empty input
func (cd ConfirmationDefault) DefaultValue() bool {
	switch cd {
	case DefaultYes:
		return true
	case DefaultNo:
		return false
	default:
		return false // fallback to safer default
	}
}

// IsConfirmed checks if the given response indicates confirmation based on the default
func (cd ConfirmationDefault) IsConfirmed(response string) bool {
	response = strings.TrimSpace(strings.ToLower(response))

	// Empty response uses the default
	if response == "" {
		return cd.DefaultValue()
	}

	// Explicit yes responses
	if response == "y" || response == "yes" {
		return true
	}

	// Explicit no responses
	if response == "n" || response == "no" {
		return false
	}

	// Any other input uses the default (safer behavior)
	return cd.DefaultValue()
}

// String returns a string representation of the ConfirmationDefault for debugging
func (cd ConfirmationDefault) String() string {
	switch cd {
	case DefaultYes:
		return "DefaultYes"
	case DefaultNo:
		return "DefaultNo"
	default:
		return "Unknown"
	}
}

// PromptForConfirmation prompts the user for a yes/no confirmation
func PromptForConfirmation(prompt string, defaultResponse ConfirmationDefault) (bool, error) {
	fmt.Print(prompt + defaultResponse.Suffix())

	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		// Handle case where user just presses Enter (no input)
		response = ""
	}

	return defaultResponse.IsConfirmed(response), nil
}

func (is *InteractiveSetup) getTLSSetup(host string) (bool, string, error) {
	if err := is.discoverer.connectivityTester.TestKubeArchiveLivezEndpoint(host, false, nil); err == nil {
		return false, "", nil
	}
	fmt.Printf("🔒 SSL/TLS connection to %s failed.\n", host)
	fmt.Println()
	fmt.Println("You have the following options:")
	fmt.Println("  1. Provide a custom certificate authority file")
	fmt.Println("  2. Use insecure TLS (skip certificate verification)")
	fmt.Println()

	choice, err := is.PromptForInput("Choose an option (1 or 2)")
	if err != nil {
		return false, "", err
	}

	choice = strings.TrimSpace(choice)
	switch choice {
	case "1":
		// Show current directory for user reference
		currentDir, _ := os.Getwd()
		fmt.Printf("Current directory: %s\n", currentDir)
		fmt.Println("Tip: You can use ~ for home directory, or relative paths")

		certPath, err := is.PromptForInput("Enter path to certificate authority file")
		if err != nil {
			return false, "", err
		}

		certPath = strings.TrimSpace(certPath)
		if certPath == "" {
			return false, "", fmt.Errorf("no certificate path provided. Cancelling setup")
		}

		expandedCertPath, _, err := LoadCertData(certPath)
		if err != nil {
			return false, "", fmt.Errorf("failed to load certificate data: %w", err)
		}
		return false, expandedCertPath, nil

	case "2":
		confirm, err := PromptForConfirmation("⚠️  This will disable SSL certificate verification. Are you sure?", DefaultNo)
		if err != nil {
			return false, "", fmt.Errorf("error reading user input: %w", err)
		}

		if !confirm {
			return false, "", fmt.Errorf("TLS configuration cancelled")
		}

		fmt.Println("⚠️  Using insecure TLS (certificate verification disabled)")
		return true, "", nil

	default:
		return false, "", fmt.Errorf("invalid choice. Please enter 1 or 2")
	}

}

// getTokenForKubeArchive handles the authentication setup for KubeArchive
func (is *InteractiveSetup) getTokenForKubeArchive(host string) (string, error) {

	var token string
	// Check if there's a token in the current kubectl config
	k8sConfig := is.configManager.GetK8sRESTConfig()
	if k8sConfig.BearerToken != "" {
		token = k8sConfig.BearerToken
	}

	if is.discoverer.connectivityTester.TestKubeArchiveConnectivity(host, true, token, nil) == nil {
		return "", nil // empty token is returned for kubectl token
	}

	// Use a token generated from a service account
	fmt.Println("   KubeArchive requires a bearer token for authentication")
	fmt.Println()

	useServiceAccount, err := PromptForConfirmation("Would you like to create a token from a service account?", DefaultYes)
	if err != nil {
		return "", fmt.Errorf("error reading user input: %w", err)
	}

	if !useServiceAccount {
		fmt.Println("Setup cancelled - configuration requires authentication.")
		return "", fmt.Errorf("no authentication configured")
	}

	defaultServiceAccount := "default"
	defaultNamespace := is.namespace

	// Prompt for service account name
	serviceAccountInput, err := is.PromptForInput(fmt.Sprintf("Service account name [%s]", defaultServiceAccount))
	if err != nil {
		return "", fmt.Errorf("error reading service account input: %w", err)
	}

	serviceAccount := serviceAccountInput
	if serviceAccount == "" {
		serviceAccount = defaultServiceAccount
	}

	// Prompt for namespace
	namespaceInput, err := is.PromptForInput(fmt.Sprintf("Namespace [%s]", defaultNamespace))
	if err != nil {
		return "", fmt.Errorf("error reading namespace input: %w", err)
	}

	namespace := namespaceInput
	if namespace == "" {
		namespace = defaultNamespace
	}

	fmt.Printf("🔑 Creating token for service account '%s' in namespace '%s'...\n", serviceAccount, namespace)

	token, err = is.discoverer.getTokenForSA(serviceAccount, namespace)

	if err != nil {
		return "", err
	}
	fmt.Println("✓ Token created successfully")
	return token, nil
}

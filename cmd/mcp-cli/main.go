// Copyright (c) 2025 SUSE LLC.
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	session *mcp.ClientSession
	ctx     context.Context
	cancel  context.CancelFunc
)

// connectToServer reads transport config from Viper and establishes an MCP session.
func connectToServer() error {
	transportType := viper.GetString("transport")
	serverAddr := viper.GetString("server")

	// If the address doesn't contain a scheme, assume http://
	if !strings.Contains(serverAddr, "://") {
		serverAddr = "http://" + serverAddr
	}

	u, err := url.Parse(serverAddr)
	if err != nil {
		return fmt.Errorf("invalid server address: %v", err)
	}

	// If no path is provided, default to /mcp
	if u.Path == "" || u.Path == "/" {
		u.Path = "/mcp"
	}

	baseURL := u.String()

	var transport mcp.Transport
	switch transportType {
	case "sse":
		transport = &mcp.SSEClientTransport{
			Endpoint: baseURL,
		}
	case "http":
		transport = &mcp.StreamableClientTransport{
			Endpoint: baseURL,
		}
	case "stdio":
		commandArgs := viper.GetStringSlice("command")
		if len(commandArgs) == 0 {
			return fmt.Errorf("command is required for stdio transport (use --command)")
		}
		serverCmd := exec.Command(commandArgs[0], commandArgs[1:]...)
		serverCmd.Stderr = os.Stderr // Redirect stderr so we can see server errors
		transport = &mcp.CommandTransport{
			Command: serverCmd,
		}
	default:
		return fmt.Errorf("invalid transport type: %s", transportType)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "simple-mcp-cli",
		Version: "1.0.0",
	}, nil)

	session, err = client.Connect(ctx, transport, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "simple-mcp-cli",
	Short: "A CLI for the simple-mcp server",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Commands with DisableFlagParsing handle their own connection setup
		// because Cobra can't parse persistent flags for them.
		if cmd.DisableFlagParsing {
			return nil
		}
		return connectToServer()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if session != nil {
			session.Close()
		}
		if cancel != nil {
			cancel()
		}
	},
}

var listToolsCmd = &cobra.Command{
	Use:   "list-tools",
	Short: "List available tools",
	Run: func(cmd *cobra.Command, args []string) {
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			log.Fatalf("Failed to list tools: %v", err)
		}
		for _, tool := range tools.Tools {
			fmt.Println(tool.Name)
		}
	},
}

var showToolCmd = &cobra.Command{
	Use:   "show-tool <tool-name>",
	Short: "Show details of a specific tool",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			log.Fatalf("Failed to list tools: %v", err)
		}
		for _, tool := range tools.Tools {
			if tool.Name == toolName {
				fmt.Println(tool.Description)
				return
			}
		}
		fmt.Printf("Tool not found: %s\n", toolName)
		os.Exit(1)
	},
}

var listResourcesCmd = &cobra.Command{
	Use:   "list-resources",
	Short: "List available resources",
	Run: func(cmd *cobra.Command, args []string) {
		resources, err := session.ListResources(ctx, nil)
		if err != nil {
			log.Fatalf("Failed to list resources: %v", err)
		}
		for _, resource := range resources.Resources {
			fmt.Println(resource.URI)
		}
	},
}

var showResourceCmd = &cobra.Command{
	Use:   "show-resource <resource-uri>",
	Short: "Show details of a specific resource",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resourceURI := args[0]
		resources, err := session.ListResources(ctx, nil)
		if err != nil {
			log.Fatalf("Failed to list resources: %v", err)
		}
		for _, resource := range resources.Resources {
			if resource.URI == resourceURI {
				fmt.Println(resource.Description)
				return
			}
		}
		fmt.Printf("Resource not found: %s\n", resourceURI)
		os.Exit(1)
	},
}

var resourceCmd = &cobra.Command{
	Use:   "resource <resource-uri>",
	Short: "Read a specific resource",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resourceURI := args[0]
		readResult, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
			URI: resourceURI,
		})
		if err != nil {
			log.Fatalf("Failed to read resource: %v", err)
		}
		for _, content := range readResult.Contents {
			if content.Text != "" {
				fmt.Print(content.Text)
			} else if content.Blob != nil {
				fmt.Print(string(content.Blob))
			}
		}
	},
}

var toolCmd = &cobra.Command{
	Use:                "tool <tool-name> [--param-name param-value]...",
	Short:              "Call a tool",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// DisableFlagParsing prevents Cobra from parsing persistent flags,
		// so we manually extract them from the raw args before connecting.
		var remaining []string
		var commandValues []string
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "-h" || arg == "--help" {
				return cmd.Help()
			}
			if !strings.HasPrefix(arg, "--") {
				remaining = append(remaining, arg)
				continue
			}
			flagName := strings.TrimPrefix(arg, "--")
			switch flagName {
			case "transport":
				if i+1 >= len(args) {
					return fmt.Errorf("missing value for --%s", flagName)
				}
				viper.Set("transport", args[i+1])
				i++
			case "server":
				if i+1 >= len(args) {
					return fmt.Errorf("missing value for --%s", flagName)
				}
				viper.Set("server", args[i+1])
				i++
			case "command":
				if i+1 >= len(args) {
					return fmt.Errorf("missing value for --%s", flagName)
				}
				commandValues = append(commandValues, strings.Split(args[i+1], ",")...)
				i++
			default:
				// Tool parameter: collect --param-name and its value
				remaining = append(remaining, arg)
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
					remaining = append(remaining, args[i+1])
					i++
				}
			}
		}
		if len(commandValues) > 0 {
			viper.Set("command", commandValues)
		}

		if err := connectToServer(); err != nil {
			return err
		}

		if len(remaining) < 1 {
			cmd.Help()
			return fmt.Errorf("tool name is required")
		}
		toolName := remaining[0]
		toolArgs := remaining[1:]
		params := make(map[string]any)
		for i := 0; i < len(toolArgs); i++ {
			if strings.HasPrefix(toolArgs[i], "--") {
				paramName := strings.TrimPrefix(toolArgs[i], "--")
				if i+1 < len(toolArgs) {
					params[paramName] = toolArgs[i+1]
					i++
				} else {
					return fmt.Errorf("missing value for parameter: %s", paramName)
				}
			}
		}

		callResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: params,
		})
		if err != nil {
			return fmt.Errorf("failed to call tool: %v", err)
		}
		if callResult.IsError {
			for _, content := range callResult.Content {
				if textContent, ok := content.(*mcp.TextContent); ok {
					log.Printf("Tool returned an error: %s", textContent.Text)
				}
			}
			return fmt.Errorf("tool returned an error")
		}
		for _, content := range callResult.Content {
			if textContent, ok := content.(*mcp.TextContent); ok {
				fmt.Print(textContent.Text)
			}
		}
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("server", "localhost:8080", "Address or URL of the MCP server (e.g. localhost:8080 or http://localhost:8080/sse).")
	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))

	rootCmd.PersistentFlags().String("transport", "sse", "Transport method to use (sse, http, stdio).")
	viper.BindPFlag("transport", rootCmd.PersistentFlags().Lookup("transport"))

	rootCmd.PersistentFlags().StringSlice("command", []string{}, "Command to run for stdio transport.")
	viper.BindPFlag("command", rootCmd.PersistentFlags().Lookup("command"))

	viper.AutomaticEnv()

	rootCmd.AddCommand(listToolsCmd)
	rootCmd.AddCommand(showToolCmd)
	rootCmd.AddCommand(listResourcesCmd)
	rootCmd.AddCommand(showResourceCmd)
	rootCmd.AddCommand(resourceCmd)
	rootCmd.AddCommand(toolCmd)
}

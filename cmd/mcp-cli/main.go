// Copyright (c) 2025 SUSE LLC.
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

var rootCmd = &cobra.Command{
	Use:   "simple-mcp-cli",
	Short: "A CLI for the simple-mcp server",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		serverAddr := viper.GetString("server")
		baseURL := fmt.Sprintf("http://%s/mcp", serverAddr)

		transport := &mcp.SSEClientTransport{
			Endpoint: baseURL,
		}

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		client := mcp.NewClient(&mcp.Implementation{
			Name:    "simple-mcp-cli",
			Version: "1.0.0",
		}, nil)

		var err error
		session, err = client.Connect(ctx, transport, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to connect to server: %v", err)
		}
		log.Printf("Connected to server.")
		return nil
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
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Help()
			os.Exit(1)
		}
		toolName := args[0]
		toolArgs := args[1:]
		params := make(map[string]any)
		for i := 0; i < len(toolArgs); i++ {
			if strings.HasPrefix(toolArgs[i], "--") {
				paramName := strings.TrimPrefix(toolArgs[i], "--")
				if i+1 < len(toolArgs) {
					params[paramName] = toolArgs[i+1]
					i++
				} else {
					log.Fatalf("Missing value for parameter: %s", paramName)
				}
			}
		}

		callResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: params,
		})
		if err != nil {
			log.Fatalf("Failed to call tool: %v", err)
		}
		if callResult.IsError {
			for _, content := range callResult.Content {
				if textContent, ok := content.(*mcp.TextContent); ok {
					log.Printf("Tool returned an error: %s", textContent.Text)
				}
			}
			log.Fatalf("Tool returned an unknown error")
		} else {
			for _, content := range callResult.Content {
				if textContent, ok := content.(*mcp.TextContent); ok {
					fmt.Print(textContent.Text)
				}
			}
		}
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("server", "localhost:8080", "Address of the simple-mcp server.")
	viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
	viper.AutomaticEnv()

	rootCmd.AddCommand(listToolsCmd)
	rootCmd.AddCommand(showToolCmd)
	rootCmd.AddCommand(listResourcesCmd)
	rootCmd.AddCommand(showResourceCmd)
	rootCmd.AddCommand(resourceCmd)
	rootCmd.AddCommand(toolCmd)
}

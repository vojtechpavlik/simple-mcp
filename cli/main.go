// Copyright (c) 2025 SUSE LLC.
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	serverAddr := flag.String("server", "localhost:8080", "Address of the simple-mcp server.")
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Println("Usage: simple-mcp-cli [options] <subcommand> [args]")
		fmt.Println("Subcommands: list-tools, show-tool, list-resources, show-resource, resource, tool")
		os.Exit(1)
	}

	baseURL := fmt.Sprintf("http://%s/mcp", *serverAddr)
	
	transport := &mcp.SSEClientTransport{
		Endpoint: baseURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "simple-mcp-cli",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer session.Close()

	log.Printf("Connected to server.")

	subcommand := flag.Arg(0)

	switch subcommand {
	case "list-tools":
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			log.Fatalf("Failed to list tools: %v", err)
		}
		for _, tool := range tools.Tools {
			fmt.Println(tool.Name)
		}
	case "show-tool":
		if len(flag.Args()) < 2 {
			fmt.Println("Usage: simple-mcp-cli show-tool <tool-name>")
			os.Exit(1)
		}
		toolName := flag.Arg(1)
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
	case "list-resources":
		resources, err := session.ListResources(ctx, nil)
		if err != nil {
			log.Fatalf("Failed to list resources: %v", err)
		}
		for _, resource := range resources.Resources {
			fmt.Println(resource.URI)
		}
	case "show-resource":
		if len(flag.Args()) < 2 {
			fmt.Println("Usage: simple-mcp-cli show-resource <resource-uri>")
			os.Exit(1)
		}
		resourceURI := flag.Arg(1)
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
	case "resource":
		if len(flag.Args()) < 2 {
			fmt.Println("Usage: simple-mcp-cli resource <resource-uri>")
			os.Exit(1)
		}
		resourceURI := flag.Arg(1)
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
	case "tool":
		if len(flag.Args()) < 2 {
			fmt.Println("Usage: simple-mcp-cli tool <tool-name> [--<param-name> <param-value>]...")
			os.Exit(1)
		}
		toolName := flag.Arg(1)
		args := flag.Args()[2:]
		params := make(map[string]any)
		for i := 0; i < len(args); i++ {
			if strings.HasPrefix(args[i], "--") {
				paramName := strings.TrimPrefix(args[i], "--")
				if i+1 < len(args) {
					params[paramName] = args[i+1]
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
	default:
		fmt.Printf("Unknown subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package main is the entry point for the simple-mcp server. It initializes the
// MCP server, sets up the async task store, parses the configuration, and
// registers all tools and resources before starting the HTTP listener.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/SUSE/simple-mcp/internal/config"
	"github.com/SUSE/simple-mcp/internal/executor"
	"github.com/SUSE/simple-mcp/internal/resource"
	"github.com/SUSE/simple-mcp/internal/scratch"
	"github.com/SUSE/simple-mcp/internal/shared"
	"github.com/SUSE/simple-mcp/internal/task"
)

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func ptr[T any](v T) *T {
	return &v
}

var (
	cfgFile string
)

var rootCmd = &cobra.Command{
	Use:   "simple-mcp",
	Short: "A simple MCP server",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(cfgFile)
		if err != nil {
			log.Fatalf("ERROR: Error loading configuration: %v", err)
		}
		log.Printf("Configuration loaded successfully from %s", cfgFile)

		// Apply configuration overrides if flags are not explicitly set
		if !cmd.Flags().Changed("listen-addr") && cfg.Specification.ListenAddr != "" {
			viper.Set("listen-addr", cfg.Specification.ListenAddr)
		}

		if !cmd.Flags().Changed("tmpdir") && cfg.Specification.TmpDir != "" {
			viper.Set("tmpdir", cfg.Specification.TmpDir)
		}

		if !cmd.Flags().Changed("verbose") && cfg.Specification.Verbose != nil {
			viper.Set("verbose", *cfg.Specification.Verbose)
		}

		if !cmd.Flags().Changed("max-async-tasks") && cfg.Specification.MaxAsyncTasks != 0 {
			viper.Set("max-async-tasks", cfg.Specification.MaxAsyncTasks)
		}

		if !cmd.Flags().Changed("transport") && cfg.Specification.Transport != "" {
			viper.Set("transport", cfg.Specification.Transport)
		}

		finalListenAddr := viper.GetString("listen-addr")
		finalTmpDir := viper.GetString("tmpdir")
		finalVerbose := viper.GetBool("verbose")
		finalMaxAsyncTasks := viper.GetInt("max-async-tasks")
		finalTransport := viper.GetString("transport")

		// Validate the transport option
		if finalTransport != "auto" && finalTransport != "sse" && finalTransport != "http" && finalTransport != "stdio" {
			log.Fatalf("ERROR: Invalid transport option '%s'. Must be one of 'auto', 'sse', 'http', or 'stdio'.", finalTransport)
		}

		if finalTmpDir != "" {
			// Verify the directory exists and is writable first.
			if err := checkTmpDir(finalTmpDir); err != nil {
				log.Fatalf("ERROR: Invalid scratch space directory: %v", err)
			}

			// Resolve it to its absolute, real physical path.
			absTmpDir, err := filepath.Abs(finalTmpDir)
			if err != nil {
				log.Fatalf("ERROR: Could not get absolute path for scratch space: %v", err)
			}
			realTmpDir, err := filepath.EvalSymlinks(absTmpDir)
			if err != nil {
				log.Fatalf("ERROR: Could not resolve symlinks for scratch space: %v", err)
			}
			finalTmpDir = realTmpDir

			log.Printf("Scratch space enabled at: %s", finalTmpDir)
		}

		taskStore := task.NewTaskStore(finalMaxAsyncTasks)
		log.Printf("Task store initialized with limit: %d", finalMaxAsyncTasks)

		// Pre-cache resource definitions for efficient lookup by the GetResource tool.
		resourceMap := make(map[string]config.ResourceItem)
		for _, item := range cfg.Specification.Resources {
			resourceMap[item.URI] = item
		}
		log.Printf("Cached %d resource definitions.", len(resourceMap))

		serverFactory := func(req *http.Request) *mcp.Server {
			s := mcp.NewServer(&mcp.Implementation{
				Name:    cfg.Metadata.Name,
				Version: cfg.APIVersion,
			}, &mcp.ServerOptions{
				Capabilities: &mcp.ServerCapabilities{
					Resources: &mcp.ResourceCapabilities{
						Subscribe:   true,
						ListChanged: true,
					},
					Tools: &mcp.ToolCapabilities{
						ListChanged: true,
					},
				},
			})
			registerBuiltinTools(s, taskStore, resourceMap, finalTmpDir, finalVerbose)
			registerConfigTools(s, cfg, taskStore, finalTmpDir, finalVerbose)
			registerResources(s, cfg, finalTmpDir, finalVerbose)
			if finalTmpDir != "" {
				scratch.RegisterScratchTools(s, resourceMap, finalTmpDir, finalVerbose)
			}
			return s
		}

		if finalTransport == "stdio" {
			s := serverFactory(nil)
			log.Printf("MCP server starting on stdio...")
			if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
				log.Fatalf("ERROR: Server error: %v", err)
			}
		} else if finalTransport == "auto" || finalTransport == "sse" || finalTransport == "http" {
			sseHandler := mcp.NewSSEHandler(serverFactory, nil)
			streamableHandler := mcp.NewStreamableHTTPHandler(serverFactory, nil)

			// The auto-detecting handler for the generic endpoint (/mcp)
			multiplexedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// SSE transport (2024 spec) starts with a GET request expecting text/event-stream
				// or a POST with a session ID for existing sessions.
				if (r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/event-stream")) ||
					(r.Method == http.MethodPost && (r.URL.Query().Get("sessionid") != "" || r.URL.Query().Get("session_id") != "")) {
					sseHandler.ServeHTTP(w, r)
					return
				}
				// Default to Streamable HTTP (2025 spec) for other requests
				streamableHandler.ServeHTTP(w, r)
			})

			var defaultHandler http.Handler
			if finalTransport == "sse" {
				defaultHandler = sseHandler
			} else if finalTransport == "http" {
				defaultHandler = streamableHandler
			} else {
				defaultHandler = multiplexedHandler
			}

			// Register standard endpoints
			log.Printf("MCP server starting (%s), listening on %s ...", finalTransport, finalListenAddr)

			// 1. Generic endpoint with auto-detection/multiplexing
			http.Handle("/mcp", defaultHandler)
			http.Handle("/mcp/", defaultHandler)

			// 2. Dedicated SSE endpoint (2024 spec)
			http.Handle("/sse", sseHandler)
			http.Handle("/sse/", sseHandler)

			// 3. Dedicated Streamable HTTP endpoint (2025 spec)
			http.Handle("/messages", streamableHandler)
			http.Handle("/messages/", streamableHandler)

			log.Printf("  - Unified/Auto-detect: %s/mcp", finalListenAddr)
			log.Printf("  - Dedicated SSE:       %s/sse", finalListenAddr)
			log.Printf("  - Dedicated Streamable:%s/messages", finalListenAddr)

			if err := http.ListenAndServe(finalListenAddr, nil); err != nil {
				log.Fatalf("ERROR: Could not start HTTP server: %v", err)
			}
		} else {
			log.Fatalf("no valid transport")
		}
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize()

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "./simple-mcp.yaml", "Path to the YAML configuration file.")

	rootCmd.Flags().String("listen-addr", "localhost:8080", "Address to listen on for HTTP requests.")
	viper.BindPFlag("listen-addr", rootCmd.Flags().Lookup("listen-addr"))

	rootCmd.Flags().String("tmpdir", "", "Path to a directory for scratch space.")
	viper.BindPFlag("tmpdir", rootCmd.Flags().Lookup("tmpdir"))

	rootCmd.Flags().Bool("verbose", false, "Enable verbose logging of MCP protocol messages.")
	viper.BindPFlag("verbose", rootCmd.Flags().Lookup("verbose"))

	rootCmd.Flags().Int("max-async-tasks", 20, "Maximum number of asynchronous tasks to keep in memory.")
	viper.BindPFlag("max-async-tasks", rootCmd.Flags().Lookup("max-async-tasks"))

	rootCmd.Flags().StringP("transport", "t", "auto", "Transport method for MCP (auto, sse, http, stdio).")
	viper.BindPFlag("transport", rootCmd.Flags().Lookup("transport"))

	viper.AutomaticEnv()
	viper.SetEnvPrefix("SIMPLE_MCP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}

func checkTmpDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("could not stat path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Check for write permissions by creating a temporary file.
	tmpFile, err := os.CreateTemp(path, "simple-mcp-write-test-")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	os.Remove(tmpFile.Name()) // Clean up the temporary file.

	return nil
}

// Input structs for builtin tools
type ListPendingTasksRequest struct{}
type TaskStatusRequest struct {
	TaskID string `json:"taskID" jsonschema:"The Task ID (e.g. task-...) or full Task URI (e.g. simple-mcp://tasks/...)"`
}
type ListResourcesToolRequest struct{}
type GetResourceToolRequest struct {
	ResourceURI string `json:"resourceURI" jsonschema:"The full URI of the resource (e.g. simple-mcp://system/uptime)."`
}
type SearchResourcesRequest struct {
	Query string `json:"query" jsonschema:"The regular expression to search for."`
}

// registerBuiltinTools adds the core infrastructure tools required for
// mcphost compatibility and async task management.
func registerBuiltinTools(mcpServer *mcp.Server, taskStore *task.TaskStore, resourceMap map[string]config.ResourceItem, tmpDir string, verbose bool) {
	// Helps the LLM recover context if it forgets a task ID.
	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "ListPendingTasks", Description: "Lists all asynchronous tasks that are currently 'pending' or 'running'."},
		func(ctx context.Context, request *mcp.CallToolRequest, req ListPendingTasksRequest) (*mcp.CallToolResult, shared.EmptyOutput, error) {
			if verbose {
				log.Printf("Handling ListPendingTasks request.")
			}
			activeTasks := taskStore.ListActiveTasks()
			if len(activeTasks) == 0 {
				return shared.NewTextResult("No active (pending or running) tasks found."), shared.EmptyOutput{}, nil
			}

			var b strings.Builder
			b.WriteString(fmt.Sprintf("Found %d active tasks:\n\n", len(activeTasks)))
			for _, task := range activeTasks {
				b.WriteString(fmt.Sprintf("Tool: %s\nTaskID: %s\nStatus: %s\nRunning For: %s\n\n",
					task.ToolName, task.ID, task.Status, time.Since(task.StartTime).Truncate(time.Second)))
			}
			return shared.NewTextResult(b.String()), shared.EmptyOutput{}, nil
		})
	log.Printf("Registered built-in tool: ListPendingTasks")

	// Polling mechanism for clients that don't support async resource subscriptions.
	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "TaskStatus", Description: "Gets the status of a long-running async task from its Task ID or URI."},
		func(ctx context.Context, request *mcp.CallToolRequest, req TaskStatusRequest) (*mcp.CallToolResult, shared.EmptyOutput, error) {
			taskID := req.TaskID
			if verbose {
				log.Printf("Handling TaskStatus request for taskID: %s", taskID)
			}

			if strings.HasPrefix(taskID, "simple-mcp://tasks/") {
				taskID = strings.TrimPrefix(taskID, "simple-mcp://tasks/")
			}

			task, ok := taskStore.Get(taskID)
			if !ok {
				log.Printf("TaskStatus request for non-existent ID: %s", taskID)
				return shared.NewTextResult(fmt.Sprintf("Status: not_found\nMessage: No task found with ID: %s", taskID)), shared.EmptyOutput{}, nil
			}

			log.Printf("Handling TaskStatus request for: %s", taskID)
			return shared.NewTextResult(task.FormatStatus()), shared.EmptyOutput{}, nil
		})
	log.Printf("Registered built-in tool: TaskStatus")

	// Provides a discoverable list of system context resources.
	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "ListResources", Description: "Lists all available system resources (context) provided by this server."},
		func(ctx context.Context, request *mcp.CallToolRequest, req ListResourcesToolRequest) (*mcp.CallToolResult, shared.EmptyOutput, error) {
			if verbose {
				log.Printf("Handling ListResources request.")
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("Found %d resources:\n\n", len(resourceMap)))
			for uri, item := range resourceMap {
				b.WriteString(fmt.Sprintf("URI: %s\nDescription: %s\n\n", uri, item.Description))
			}
			return shared.NewTextResult(b.String()), shared.EmptyOutput{}, nil
		})
	log.Printf("Registered built-in tool: ListResources")

	// Allows retrieving resource content via a tool call, bypassing client-side restrictions.
	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "GetResource", Description: "Gets the current content of a specific resource by its URI."},
		func(ctx context.Context, request *mcp.CallToolRequest, req GetResourceToolRequest) (*mcp.CallToolResult, shared.EmptyOutput, error) {
			resourceURI := req.ResourceURI
			if verbose {
				log.Printf("Handling GetResource request for: %s", resourceURI)
			}

			item, ok := resourceMap[resourceURI]
			if !ok {
				return shared.NewErrorResult(fmt.Sprintf("Resource not found: %s. Call ListResources to see available URIs.", resourceURI)), shared.EmptyOutput{}, nil
			}

			content, err := resource.GetResourceContent(item, tmpDir, verbose)
			if err != nil {
				log.Printf("ERROR: Unexpected error getting resource content for %s: %v", resourceURI, err)
				return shared.NewErrorResult(fmt.Sprintf("Unexpected error getting resource content for %s: %v", resourceURI, err)), shared.EmptyOutput{}, nil
			}

			return shared.NewTextResult(content), shared.EmptyOutput{}, nil
		})
	log.Printf("Registered built-in tool: GetResource")

	// Allows searching through resource definitions.
	mcp.AddTool(mcpServer,
		&mcp.Tool{Name: "SearchResources", Description: "Searches for resources by URI, description, or static content using a regular expression."},
		func(ctx context.Context, request *mcp.CallToolRequest, req SearchResourcesRequest) (*mcp.CallToolResult, shared.EmptyOutput, error) {
			query := req.Query
			if verbose {
				log.Printf("Handling SearchResources request with query: %s", query)
			}

			result, err := searchResources(resourceMap, query)
			if err != nil {
				return shared.NewErrorResult(err.Error()), shared.EmptyOutput{}, nil
			}

			return shared.NewTextResult(result), shared.EmptyOutput{}, nil
		})
	log.Printf("Registered built-in tool: SearchResources")
}

// searchResources filters the resourceMap based on a regular expression query
// matching against URI, description, or static content.
func searchResources(resourceMap map[string]config.ResourceItem, query string) (string, error) {
	re, err := regexp.Compile(query)
	if err != nil {
		return "", fmt.Errorf("invalid regular expression: %v", err)
	}

	var matched []string
	for uri, item := range resourceMap {
		if re.MatchString(uri) || re.MatchString(item.Description) || re.MatchString(item.Content) {
			matched = append(matched, uri)
		}
	}
	sort.Strings(matched)

	if len(matched) == 0 {
		return "No resources matched the search query.", nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d matching resources:\n\n", len(matched)))
	for _, uri := range matched {
		item := resourceMap[uri]
		b.WriteString(fmt.Sprintf("URI: %s\nDescription: %s\n\n", uri, item.Description))
	}
	return b.String(), nil
}


// generateSchema creates a JSON Schema object for the tool parameters.
func generateSchema(params []config.Parameter) map[string]interface{} {
	properties := make(map[string]interface{})
	required := []string{}

	for _, param := range params {
		properties[param.Name] = map[string]interface{}{
			"type":        "string", // We treat everything as string for simplicity in cli args, but could be specific
			"description": fmt.Sprintf("Parameter: %s", param.Name),
		}
		required = append(required, param.Name)
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
	return schema
}

// registerConfigTools iterates through the configuration and registers
// declared tools, routing them to sync or async handlers.
func registerConfigTools(mcpServer *mcp.Server, cfg *config.Config, taskStore *task.TaskStore, tmpDir string, verbose bool) {
	for _, item := range cfg.Specification.Tools {
		currentItem := item

		// Manual schema generation
		inputSchema := generateSchema(item.Parameters)

		toolDef := &mcp.Tool{
			Name:        item.Name,
			Description: item.Description,
			InputSchema: inputSchema,
		}

		mcp.AddTool(mcpServer, toolDef, func(ctx context.Context, request *mcp.CallToolRequest, params map[string]interface{}) (*mcp.CallToolResult, shared.EmptyOutput, error) {
			log.Printf("Handling request for tool: %s", currentItem.Name)

			// Validate parameters
			for _, param := range currentItem.Parameters {
				valRaw, ok := params[param.Name]
				if !ok {
					return shared.NewErrorResult(fmt.Sprintf("Missing parameter: %s", param.Name)), shared.EmptyOutput{}, nil
				}
				val, ok := valRaw.(string)
				if !ok {
					// Try to convert non-string to string
					val = fmt.Sprintf("%v", valRaw)
				}

				if err := validateParameter(param, val, tmpDir); err != nil {
					return shared.NewErrorResult(err.Error()), shared.EmptyOutput{}, nil
				}
				// Ensure params map has the string value
				params[param.Name] = val
			}

			if verbose {
				log.Printf("Tool parameters: %v", params)
			}

			if currentItem.Async {
				return handleAsyncTask(ctx, mcpServer, currentItem, params, taskStore, tmpDir, verbose)
			}
			return handleSyncTask(ctx, currentItem, params, tmpDir, verbose)
		})

		logMessage := fmt.Sprintf("Registered tool: %s", item.Name)
		if item.Async {
			logMessage += " (Async)"
		}
		if item.TimeoutSeconds > 0 {
			logMessage += fmt.Sprintf(" (Timeout: %ds)", item.TimeoutSeconds)
		}
		log.Println(logMessage)
	}
}

// validateParameter checks if a parameter value satisfies the specified type
// and validation regular expression.
func validateParameter(p config.Parameter, value string, tmpDir string) error {
	// 1. Check regexp if specified
	if p.Validator != "" {
		re, err := regexp.Compile(p.Validator)
		if err != nil {
			return fmt.Errorf("invalid validator regexp for parameter '%s': %v", p.Name, err)
		}
		if !re.MatchString(value) {
			return fmt.Errorf("parameter '%s' failed regexp check", p.Name)
		}
	}

	// 2. Check type
	switch p.Type {
	case "":
		// No type specified, skip
	case "path":
		if err := checkShellSafe(value); err != nil {
			return fmt.Errorf("parameter '%s' is not a valid path: %v", p.Name, err)
		}
	case "filename":
		if strings.ContainsAny(value, "/\\") {
			return fmt.Errorf("parameter '%s' must not contain path separators", p.Name)
		}
		if err := checkShellSafe(value); err != nil {
			return fmt.Errorf("parameter '%s' is not a valid filename: %v", p.Name, err)
		}
	case "number":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("parameter '%s' must be a valid number", p.Name)
		}
	case "integer":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("parameter '%s' must be a valid integer", p.Name)
		}
	case "word":
		if matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", value); !matched {
			return fmt.Errorf("parameter '%s' must be an alphanumeric word without spaces", p.Name)
		}
	case "directory":
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("parameter '%s': path does not exist or is not accessible: %v", p.Name, value)
		}
		if !info.IsDir() {
			return fmt.Errorf("parameter '%s': path is not a directory: %v", p.Name, value)
		}
	case "file":
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("parameter '%s': path does not exist or is not accessible: %v", p.Name, value)
		}
		if info.IsDir() {
			return fmt.Errorf("parameter '%s': path is a directory, expected a file: %v", p.Name, value)
		}
	case "tmpDir":
		if tmpDir == "" {
			return fmt.Errorf("parameter '%s' uses 'tmpDir' type but scratch space is not enabled", p.Name)
		}
		fullPath, err := shared.ResolvePath(tmpDir, value)
		if err != nil {
			return fmt.Errorf("parameter '%s': invalid scratch space path: %v", p.Name, err)
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("parameter '%s': path does not exist in scratch space: %v", p.Name, value)
		}
		if !info.IsDir() {
			return fmt.Errorf("parameter '%s': path is not a directory in scratch space: %v", p.Name, value)
		}
	case "tmpFile":
		if tmpDir == "" {
			return fmt.Errorf("parameter '%s' uses 'tmpFile' type but scratch space is not enabled", p.Name)
		}
		fullPath, err := shared.ResolvePath(tmpDir, value)
		if err != nil {
			return fmt.Errorf("parameter '%s': invalid scratch space path: %v", p.Name, err)
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("parameter '%s': path does not exist in scratch space: %v", p.Name, value)
		}
		if info.IsDir() {
			return fmt.Errorf("parameter '%s': path is a directory in scratch space, expected a file: %v", p.Name, value)
		}
	default:
		return fmt.Errorf("unknown parameter type '%s' for parameter '%s'", p.Type, p.Name)
	}

	return nil
}

// checkShellSafe verifies that a string does not contain unescaped spaces or
// shell control characters.
func checkShellSafe(s string) error {
	dangerous := ";&|<>$( )`\"'*?[]!{}"
	escaped := false
	for _, r := range s {
		if r == '\\' {
			escaped = !escaped
			continue
		}
		if strings.ContainsRune(dangerous, r) {
			if !escaped {
				return fmt.Errorf("contains unescaped shell character '%c'", r)
			}
		}
		escaped = false
	}
	if escaped {
		return fmt.Errorf("trailing backslash")
	}
	return nil
}

func handleSyncTask(ctx context.Context, currentItem config.ContextItem, params map[string]interface{}, tmpDir string, verbose bool) (*mcp.CallToolResult, shared.EmptyOutput, error) {
	output, err := executor.ExecuteCommand(currentItem, params, tmpDir)
	if err != nil {
		log.Printf("ERROR: Error executing command '%s' (Exit Code: %d): %v", currentItem.Name, output.ReturnCode, err)
		// Return stderr output to the LLM to help with diagnosing the failure.
		return shared.NewErrorResult(fmt.Sprintf("Command failed: %v. Output: %s", err, output.Result)), shared.EmptyOutput{}, nil
	}

	log.Printf("Successfully executed tool '%s', output: %d bytes, %d lines, exit code: %d, duration: %s", currentItem.Name, len(output.Result), countLines(output.Result), output.ReturnCode, output.Duration)
	return shared.NewTextResult(output.Result), shared.EmptyOutput{}, nil
}
func handleAsyncTask(ctx context.Context, srv *mcp.Server, currentItem config.ContextItem, params map[string]interface{}, taskStore *task.TaskStore, tmpDir string, verbose bool) (*mcp.CallToolResult, shared.EmptyOutput, error) {
	// Enforce concurrency lock: prevent multiple instances of the same long-running task.
	if taskStore.HasActiveTask(currentItem.Name) {
		log.Printf("Rejected async task %s: task is already running.", currentItem.Name)
		return shared.NewErrorResult(fmt.Sprintf("Task '%s' is already in progress. Call 'ListPendingTasks' or 'TaskStatus' to monitor it.", currentItem.Name)), shared.EmptyOutput{}, nil
	}

	evictID, err := taskStore.PrepareSlot()
	if err != nil {
		return shared.NewErrorResult(err.Error()), shared.EmptyOutput{}, nil
	}

	if evictID != "" {
		log.Printf("Evicting oldest task: %s", evictID)
		evictURI := fmt.Sprintf("simple-mcp://tasks/%s", evictID)
		srv.RemoveResources(evictURI)
		taskStore.Delete(evictID)
	}

	jobID := task.GenerateTaskID(currentItem.Name)
	taskURI := fmt.Sprintf("simple-mcp://tasks/%s", jobID)

	task := taskStore.Create(jobID, currentItem.Name)

	// Create a dynamic resource for this specific task ID.
	taskResource := &mcp.Resource{
		Name:        fmt.Sprintf("Status of async job: %s (Job ID: %s)", currentItem.Name, jobID),
		URI:         taskURI,
		MIMEType:    "text/plain",
		Description: fmt.Sprintf("Status of async job: %s", currentItem.Name),
	}

	srv.AddResource(taskResource, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		log.Printf("Handling standard MCP resource read for task: %s", jobID)
		task, ok := taskStore.Get(jobID)
		if !ok {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						URI:      taskURI,
						MIMEType: "text/plain",
						Text:     "Status: unknown\nMessage: Task ID not found.",
					},
				},
			}, nil
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      taskURI,
					MIMEType: "text/plain",
					Text:     task.FormatStatus(),
				},
			},
		}, nil
	})

	go func() {
		// Ensure this goroutine does not crash the main server.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("ERROR: FATAL PANIC in async job %s: %v", jobID, r)
				errMsg := fmt.Sprintf("Async job %s failed with an internal server panic: %v", jobID, r)
				taskStore.SetStatus(jobID, "failed", errMsg)
			}
		}()

		log.Printf("Starting async job %s: %s", jobID, currentItem.Name)
		taskStore.SetStatus(jobID, "running", "Job is executing...")

		output, err := executor.ExecuteCommand(currentItem, params, tmpDir)

		if err != nil {
			log.Printf("ERROR: Async job %s finished with status: failed (Exit Code: %d)", jobID, output.ReturnCode)
			errMsg := fmt.Sprintf("%v. Output: %s", err, output.Result)
			taskStore.SetStatus(jobID, "failed", errMsg)
		} else {
			log.Printf("Async job %s finished with status: completed, output: %d bytes, %d lines, exit code: %d, duration: %s", jobID, len(output.Result), countLines(output.Result), output.ReturnCode, output.Duration)
			taskStore.SetStatus(jobID, "completed", output.Result)
		}
	}()

	log.Printf("Async tool %s started. Task URI: %s", currentItem.Name, taskURI)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: task.FormatStatus(),
			},
		},
	}, shared.EmptyOutput{}, nil
}

// registerResources registers the static or dynamic resources defined in the
// config file. These are separate from the ephemeral task resources.
func registerResources(mcpServer *mcp.Server, cfg *config.Config, tmpDir string, verbose bool) {
	for _, item := range cfg.Specification.Resources {
		currentItem := item

		res := &mcp.Resource{
			Name:        currentItem.Description, // Name is roughly description or URI?
			URI:         currentItem.URI,
			MIMEType:    "text/plain",
			Description: currentItem.Description,
		}

		// Combined handler for content, contentFile, and command
		mcpServer.AddResource(res, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			if verbose {
				log.Printf("Handling resource read request for: %s", currentItem.URI)
			}

			content, err := resource.GetResourceContent(currentItem, tmpDir, verbose)
			if err != nil {
				log.Printf("ERROR: Unexpected error getting resource content for %s: %v", currentItem.URI, err)
				content = fmt.Sprintf("Unexpected error getting resource content for %s: %v", currentItem.URI, err)
			}

			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						URI:      currentItem.URI,
						MIMEType: "text/plain",
						Text:     content,
					},
				},
			}, nil
		})
		log.Printf("Registered resource: %s (dynamic: %v)", currentItem.URI, currentItem.Command != "")
	}
}

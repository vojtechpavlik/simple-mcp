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
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func resolveOptions(cfg *Config, cliListenAddr string, cliTmpDir string, cliVerbose bool, cliMaxAsyncTasks int, setFlags map[string]bool) (string, string, bool, int) {
	// Precedence: Command-line flag > YAML config > Default.
	finalListenAddr := cliListenAddr
	if !setFlags["listen-addr"] && cfg.Specification.ListenAddr != "" {
		finalListenAddr = cfg.Specification.ListenAddr
	}

	finalTmpDir := cliTmpDir
	if !setFlags["tmpdir"] && cfg.Specification.TmpDir != "" {
		finalTmpDir = cfg.Specification.TmpDir
	}

	finalVerbose := cliVerbose
	if !setFlags["verbose"] && cfg.Specification.Verbose != nil {
		finalVerbose = *cfg.Specification.Verbose
	}

	finalMaxAsyncTasks := cliMaxAsyncTasks
	if !setFlags["max-async-tasks"] && cfg.Specification.MaxAsyncTasks != 0 {
		finalMaxAsyncTasks = cfg.Specification.MaxAsyncTasks
	}

	return finalListenAddr, finalTmpDir, finalVerbose, finalMaxAsyncTasks
}

func main() {
	configFile := flag.String("config", "./simple-mcp.yaml", "Path to the YAML configuration file.")
	listenAddr := flag.String("listen-addr", "localhost:8080", "Address to listen on for HTTP requests.")
	tmpDir := flag.String("tmpdir", "", "Path to a directory for scratch space.")
	verbose := flag.Bool("verbose", false, "Enable verbose logging of MCP protocol messages.")
	maxAsyncTasks := flag.Int("max-async-tasks", 20, "Maximum number of asynchronous tasks to keep in memory.")
	flag.Parse()

	cfg, err := LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("ERROR: Error loading configuration: %v", err)
	}
	log.Printf("Configuration loaded successfully from %s", *configFile)

	// Determine which flags were explicitly set by the user on the command line.
	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	finalListenAddr, finalTmpDir, finalVerbose, finalMaxAsyncTasks := resolveOptions(cfg, *listenAddr, *tmpDir, *verbose, *maxAsyncTasks, setFlags)

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

	taskStore := NewTaskStore(finalMaxAsyncTasks)
	log.Printf("Task store initialized with limit: %d", finalMaxAsyncTasks)

	// Pre-cache resource definitions for efficient lookup by the GetResource tool.
	resourceMap := make(map[string]ResourceItem)
	for _, item := range cfg.Specification.Resources {
		resourceMap[item.URI] = item
	}
	log.Printf("Cached %d resource definitions.", len(resourceMap))

	mcpServer := server.NewMCPServer(
		cfg.Metadata.Name,
		cfg.APIVersion,
		server.WithToolCapabilities(false),
		server.WithRecovery(),                       // Gracefully handle panics in handlers
		server.WithResourceCapabilities(true, true), // Advertise resource support
	)
	log.Printf("MCP Server %s with API %s created.", cfg.Metadata.Name, cfg.APIVersion)

	registerBuiltinTools(mcpServer, taskStore, resourceMap, finalTmpDir, finalVerbose)
	registerConfigTools(mcpServer, cfg, taskStore, finalTmpDir, finalVerbose)
	registerResources(mcpServer, cfg, finalTmpDir, finalVerbose)

	if finalTmpDir != "" {
		registerScratchTools(mcpServer, resourceMap, finalTmpDir, finalVerbose)
	}

	log.Printf("Creating Streamable HTTP server...")
	httpOpts := []server.StreamableHTTPOption{}
	httpServer := server.NewStreamableHTTPServer(mcpServer, httpOpts...)

	log.Printf("MCP server starting, listening on %s/mcp ...", finalListenAddr)
	if err := httpServer.Start(finalListenAddr); err != nil {
		log.Fatalf("ERROR: Could not start HTTP server: %v", err)
	}
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


// registerBuiltinTools adds the core infrastructure tools required for
// mcphost compatibility and async task management.
func registerBuiltinTools(mcpServer *server.MCPServer, taskStore *TaskStore, resourceMap map[string]ResourceItem, tmpDir string, verbose bool) {
	// Helps the LLM recover context if it forgets a task ID.
	listTasksTool := mcp.NewTool(
		"ListPendingTasks",
		mcp.WithDescription("Lists all asynchronous tasks that are currently 'pending' or 'running'."),
	)
	mcpServer.AddTool(listTasksTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if verbose {
			log.Printf("Handling ListPendingTasks request.")
		}
		activeTasks := taskStore.ListActiveTasks()
		if len(activeTasks) == 0 {
			return mcp.NewToolResultText("No active (pending or running) tasks found."), nil
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Found %d active tasks:\n\n", len(activeTasks)))
		for _, task := range activeTasks {
			b.WriteString(fmt.Sprintf("Tool: %s\nTaskID: %s\nStatus: %s\nRunning For: %s\n\n",
				task.ToolName, task.ID, task.Status, time.Since(task.StartTime).Truncate(time.Second)))
		}
		return mcp.NewToolResultText(b.String()), nil
	})
	log.Printf("Registered built-in tool: %s", listTasksTool.Name)

	// Polling mechanism for clients that don't support async resource subscriptions.
	taskStatusTool := mcp.NewTool(
		"TaskStatus",
		mcp.WithDescription("Gets the status of a long-running async task from its Task ID or URI."),
		mcp.WithString(
			"taskID",
			mcp.Required(),
			mcp.Description("The Task ID (e.g., task-...) or full Task URI (e.g., simple-mcp://tasks/...)"),
		),
	)
	mcpServer.AddTool(taskStatusTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, _ := request.RequireString("taskID")

		if verbose {
			log.Printf("Handling TaskStatus request for taskID: %s", taskID)
		}

		if strings.HasPrefix(taskID, "simple-mcp://tasks/") {
			taskID = strings.TrimPrefix(taskID, "simple-mcp://tasks/")
		}

		task, ok := taskStore.Get(taskID)
		if !ok {
			log.Printf("TaskStatus request for non-existent ID: %s", taskID)
			return mcp.NewToolResultText(fmt.Sprintf("Status: not_found\nMessage: No task found with ID: %s", taskID)), nil
		}

		log.Printf("Handling TaskStatus request for: %s", taskID)
		return mcp.NewToolResultText(task.FormatStatus()), nil
	})
	log.Printf("Registered built-in tool: %s", taskStatusTool.Name)

	// Provides a discoverable list of system context resources.
	listResourcesTool := mcp.NewTool(
		"ListResources",
		mcp.WithDescription("Lists all available system resources (context) provided by this server."),
	)
	mcpServer.AddTool(listResourcesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if verbose {
			log.Printf("Handling ListResources request.")
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Found %d resources:\n\n", len(resourceMap)))
		for uri, item := range resourceMap {
			b.WriteString(fmt.Sprintf("URI: %s\nDescription: %s\n\n", uri, item.Description))
		}
		return mcp.NewToolResultText(b.String()), nil
	})
	log.Printf("Registered built-in tool: %s", listResourcesTool.Name)

	// Allows retrieving resource content via a tool call, bypassing client-side restrictions.
	getResourceTool := mcp.NewTool(
		"GetResource",
		mcp.WithDescription("Gets the current content of a specific resource by its URI."),
		mcp.WithString(
			"resourceURI",
			mcp.Required(),
			mcp.Description("The full URI of the resource (e.g., simple-mcp://system/uptime)."),
		),
	)
	mcpServer.AddTool(getResourceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resourceURI, _ := request.RequireString("resourceURI")
		if verbose {
			log.Printf("Handling GetResource request for: %s", resourceURI)
		}

		item, ok := resourceMap[resourceURI]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("Resource not found: %s. Call ListResources to see available URIs.", resourceURI)), nil
		}

		content, err := getResourceContent(item, tmpDir, verbose)
		if err != nil {
			// getResourceContent should not return errors, but we handle it just in case.
			log.Printf("ERROR: Unexpected error getting resource content for %s: %v", resourceURI, err)
			return mcp.NewToolResultError(fmt.Sprintf("Unexpected error getting resource content for %s: %v", resourceURI, err)), nil
		}

		return mcp.NewToolResultText(content), nil
	})
	log.Printf("Registered built-in tool: %s", getResourceTool.Name)

	// Allows searching through resource definitions.
	searchResourcesTool := mcp.NewTool(
		"SearchResources",
		mcp.WithDescription("Searches for resources by URI, description, or static content using a regular expression."),
		mcp.WithString(
			"query",
			mcp.Required(),
			mcp.Description("The regular expression to search for."),
		),
	)
	mcpServer.AddTool(searchResourcesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := request.RequireString("query")
		if verbose {
			log.Printf("Handling SearchResources request with query: %s", query)
		}

		result, err := searchResources(resourceMap, query)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(result), nil
	})
	log.Printf("Registered built-in tool: %s", searchResourcesTool.Name)
}

// searchResources filters the resourceMap based on a regular expression query
// matching against URI, description, or static content.
func searchResources(resourceMap map[string]ResourceItem, query string) (string, error) {
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

// getResourceContent generates the content for a given resource, handling static content,
// dynamic command execution, and the combination of both.
func getResourceContent(item ResourceItem, tmpDir string, verbose bool) (string, error) {
	var combinedContent strings.Builder

	// Append static content first
	if item.Content != "" {
		combinedContent.WriteString(item.Content)
	}

	// Then, append command output if a command is defined
	if item.Command != "" {
		cmdItem := ContextItem{Command: item.Command}
		output, exitCode, duration, err := executeCommand(cmdItem, nil, tmpDir)

		if err != nil {
			log.Printf("ERROR: Error executing command for resource %s (Exit Code: %d): %v", item.URI, exitCode, err)
			// Append error to content for visibility to the LLM
			output = fmt.Sprintf("\nError executing command: %v. Output: %s", err, output)
		} else {
			if verbose {
				log.Printf("Successfully executed command for resource %s, output: %d bytes, %d lines, exit code: %d, duration: %s", item.URI, len(output), countLines(output), exitCode, duration)
			}
		}
		combinedContent.WriteString(output)
	}

	return combinedContent.String(), nil
}

// registerConfigTools iterates through the configuration and registers
// declared tools, routing them to sync or async handlers.
func registerConfigTools(mcpServer *server.MCPServer, cfg *Config, taskStore *TaskStore, tmpDir string, verbose bool) {
	for _, item := range cfg.Specification.Tools {
		currentItem := item
		var toolOptions []mcp.ToolOption
		toolOptions = append(toolOptions, mcp.WithDescription(item.Description))

		for _, param := range item.Parameters {
			toolOptions = append(toolOptions, mcp.WithString(
				param.Name,
				mcp.Required(),
				mcp.Description(fmt.Sprintf("Parameter: %s", param.Name)),
			))
		}

		tool := mcp.NewTool(item.Name, toolOptions...)

		handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			log.Printf("Handling request for tool: %s", currentItem.Name)

			params := make(map[string]interface{})
			for _, param := range currentItem.Parameters {
				val, err := request.RequireString(param.Name)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if err := validateParameter(param, val, tmpDir); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				params[param.Name] = val
			}

			if verbose {
				log.Printf("Tool parameters: %v", params)
			}

			if currentItem.Async {
				return handleAsyncTask(ctx, currentItem, params, taskStore, tmpDir, verbose)
			}
			return handleSyncTask(ctx, currentItem, params, tmpDir, verbose)
		}

		mcpServer.AddTool(tool, handler)

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
func validateParameter(p Parameter, value string, tmpDir string) error {
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
		fullPath, err := resolvePath(tmpDir, value)
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
		fullPath, err := resolvePath(tmpDir, value)
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

func handleSyncTask(ctx context.Context, currentItem ContextItem, params map[string]interface{}, tmpDir string, verbose bool) (*mcp.CallToolResult, error) {
	output, exitCode, duration, err := executeCommand(currentItem, params, tmpDir)
	if err != nil {
		log.Printf("ERROR: Error executing command '%s' (Exit Code: %d): %v", currentItem.Name, exitCode, err)
		// Return stderr output to the LLM to help with diagnosing the failure.
		return mcp.NewToolResultError(fmt.Sprintf("Command failed: %v. Output: %s", err, output)), nil
	}

	log.Printf("Successfully executed tool '%s', output: %d bytes, %d lines, exit code: %d, duration: %s", currentItem.Name, len(output), countLines(output), exitCode, duration)
	return mcp.NewToolResultText(output), nil
}

func handleAsyncTask(ctx context.Context, currentItem ContextItem, params map[string]interface{}, taskStore *TaskStore, tmpDir string, verbose bool) (*mcp.CallToolResult, error) {
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		log.Println("Error: could not get server from context for async task")
		return mcp.NewToolResultError("could not get server from context"), nil
	}

	// Enforce concurrency lock: prevent multiple instances of the same long-running task.
	if taskStore.HasActiveTask(currentItem.Name) {
		log.Printf("Rejected async task %s: task is already running.", currentItem.Name)
		return mcp.NewToolResultError(fmt.Sprintf("Task '%s' is already in progress. Call 'ListPendingTasks' or 'TaskStatus' to monitor it.", currentItem.Name)), nil
	}

	evictID, err := taskStore.PrepareSlot()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if evictID != "" {
		log.Printf("Evicting oldest task: %s", evictID)
		evictURI := fmt.Sprintf("simple-mcp://tasks/%s", evictID)
		srv.RemoveResource(evictURI)
		taskStore.Delete(evictID)
	}

	jobID := GenerateTaskID(currentItem.Name)
	taskURI := fmt.Sprintf("simple-mcp://tasks/%s", jobID)

	task := taskStore.Create(jobID, currentItem.Name)

	// Create a dynamic resource for this specific task ID. This follows the
	// standard MCP pattern where a task becomes a subscribable resource.
	taskResource := mcp.NewResource(
		taskURI,
		fmt.Sprintf("Status of async job: %s (Job ID: %s)", currentItem.Name, jobID),
		mcp.WithMIMEType("text/plain"),
	)
	taskResourceHandler := func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		log.Printf("Handling standard MCP resource read for task: %s", jobID)
		task, ok := taskStore.Get(jobID)
		if !ok {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      taskURI,
					MIMEType: "text/plain",
					Text:     "Status: unknown\nMessage: Task ID not found.",
				},
			}, nil
		}

		contents := []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      taskURI,
				MIMEType: "text/plain",
				Text:     task.FormatStatus(),
			},
		}
		return contents, nil
	}

	srv.AddResource(taskResource, taskResourceHandler)

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

		output, exitCode, duration, err := executeCommand(currentItem, params, tmpDir)

		if err != nil {
			log.Printf("ERROR: Async job %s finished with status: failed (Exit Code: %d)", jobID, exitCode)
			errMsg := fmt.Sprintf("%v. Output: %s", err, output)
			taskStore.SetStatus(jobID, "failed", errMsg)
		} else {
			log.Printf("Async job %s finished with status: completed, output: %d bytes, %d lines, exit code: %d, duration: %s", jobID, len(output), countLines(output), exitCode, duration)
			taskStore.SetStatus(jobID, "completed", output)
		}
	}()

	log.Printf("Async tool %s started. Task URI: %s", currentItem.Name, taskURI)
	initialContents := mcp.TextResourceContents{
		URI:      taskURI,
		MIMEType: "text/plain",
		Text:     task.FormatStatus(),
	}
	return mcp.NewToolResultResource(taskURI, initialContents), nil
}

// registerResources registers the static or dynamic resources defined in the
// config file. These are separate from the ephemeral task resources.
func registerResources(mcpServer *server.MCPServer, cfg *Config, tmpDir string, verbose bool) {
	for _, item := range cfg.Specification.Resources {
		currentItem := item

		resource := mcp.NewResource(
			currentItem.URI,
			currentItem.Description,
			mcp.WithResourceDescription(currentItem.Description),
			mcp.WithMIMEType("text/plain"),
		)

		var handler server.ResourceHandlerFunc

		// Combined handler for content, contentFile, and command
		handler = func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			if verbose {
				log.Printf("Handling resource read request for: %s", currentItem.URI)
			}

			content, err := getResourceContent(currentItem, tmpDir, verbose)
			if err != nil {
				// This path should not be reached given the current implementation of getResourceContent,
				// but is included for robustness.
				log.Printf("ERROR: Unexpected error getting resource content for %s: %v", currentItem.URI, err)
				content = fmt.Sprintf("Unexpected error getting resource content for %s: %v", currentItem.URI, err)
			}

			contents := []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      currentItem.URI,
					MIMEType: "text/plain",
					Text:     content,
				},
			}
			return contents, nil
		}
		log.Printf("Registered resource: %s (dynamic: %v)", currentItem.URI, currentItem.Command != "")

		mcpServer.AddResource(resource, handler)
	}
}

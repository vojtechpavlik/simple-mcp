# **Simple shell-interface MCP Server (simple-mcp)**

simple-mcp is a Model Context Protocol (MCP) server designed interface basic
Linux shell command line commands to LLM-based agents. It is built using Go and
the [mcp-go](https://github.com/mark3labs/mcp-go) library.

## **Prerequisites**

* **Go 1.23+** (for building)
* **mcphost** (or another MCP-compliant client)
* **Ollama** (or another LLM provider) running a capable model
  (e.g., qwen3-coder, llama3).

## **Building**

To build the server and CLI binary:

make

This will produce the `simple-mcp` and `simple-mcp-cli` binaries in the
current directory.

## **Command Line Options**

* `-config <path>`: Path to the YAML configuration file
  (default: `./simple-mcp.yaml`).
* `-listen-addr <address>`: The network address and port to listen on
  (default: `localhost:8080`).
* `-tmpdir <path>`: Path to a directory for the scratch space. Enabling this
  enables file manipulation tools.
* `-verbose`: Enable verbose logging of MCP protocol messages.
* `-max-async-tasks <number>`: Maximum number of asynchronous tasks to keep in
  memory (default: 20).

## **Configuration**

The server is configured via `simple-mcp.yaml`. The `spec` section supports the
following global options:

* `listenAddr`: Same as `-listen-addr`.
* `tmpDir`: Same as `-tmpdir`.
* `verbose`: Same as `-verbose`.
* `maxAsyncTasks`: Same as `-max-async-tasks`.

The `spec` section also defines:

* **Resources:** Data endpoints the LLM can read.
  * `uri`: The unique identifier for the resource.
  * `description`: A human-readable description.
  * `content`: Static text content.
  * `contentFile`: Path to a file whose content will be loaded (relative to the
    config file).
  * `directory`: Path to a directory whose entire subtree of files will be added
    as individual resources. The file's relative path will be appended to the
    resource URI.
  * `command`: A shell command to execute to generate dynamic content.
  * `intervalSeconds`: Suggested refresh interval for the client.
* **Tools:** Executable commands exposed to the LLM.
  * `name`: The name of the tool.
  * `description`: What the tool does.
  * `command`: The shell command to run. Supports Go template syntax
    (e.g., `{{.paramName}}`).
  * `parameters`: A list of parameter names the tool accepts.
  * `async`: If true, the tool runs in the background and returns a task URI for
    monitoring.
  * `timeoutSeconds`: Maximum execution time for the command (default: 30s).

### **Generating Configurations with LLMs**

You can use an LLM to generate secure, hardened configurations for `simple-mcp`.
The [`llm-config-generation/`](./llm-config-generation/) directory contains a
specialized system prompt and examples that can turn an LLM into a "Security
Architect" for your MCP setup.

See the [llm-config-generation README](./llm-config-generation/README.md) for
detailed instructions.

### **Parameter Validation**

Tool parameters can be validated before execution by specifying their `type`
and/or a `validator` regular expression:

* **`type`**:
  * `path`: A valid shell path (no unescaped spaces or control characters).
  * `directory`: An existing directory on the host.
  * `file`: An existing file on the host.
  * `number`: A valid number.
  * `integer`: A valid integer.
  * `word`: An alphanumeric string without spaces.
  * `filename`: A valid filename without path separators.
  * `tmpDir`: An existing directory within the scratch space.
  * `tmpFile`: An existing file within the scratch space.
* **`validator`**: A regular expression that the parameter value must match.

Example:

```yaml
  tools:
    - name: GetServiceLogs
      command: "journalctl -u {{.serviceName}} -n 10"
      parameters:
        - name: serviceName
          type: word
          validator: "^[a-z0-9.-]+$"
```

## **Built-in Capabilities**

* **Resource Search:** The server provides a built-in `SearchResources` tool
  that allows the LLM to search through all resource URIs, descriptions, and
  static content using regular expressions.
* **Async Tasks:** Tools marked as `async: true` will run in the background.
  The server provides `ListPendingTasks` and `TaskStatus` tools to monitor
  these jobs. The total number of tasks in memory is limited by `maxAsyncTasks`.
  If the limit is reached, starting a new task will evict the oldest completed
  or failed task. If all slots are filled with active (pending or running)
  tasks, new asynchronous tasks will fail until a slot becomes available.

## **Scratch Space**

When the `-tmpdir` flag or `tmpDir` configuration option is set, `simple-mcp`
provides a set of tools to the LLM for manipulating files within that
directory. This includes creating, reading, deleting, and modifying files
(using regex search-and-replace), as well as copying resources into the
scratch space.

## **CLI Tool (simple-mcp-cli)**

A command-line client is provided for testing and interacting with the server:

* `simple-mcp-cli list-tools`: List all available tools.
* `simple-mcp-cli show-tool <name>`: Show description of a tool.
* `simple-mcp-cli list-resources`: List all available resources.
* `simple-mcp-cli show-resource <uri>`: Show description of a resource.
* `simple-mcp-cli resource <uri>`: Read the content of a resource.
* `simple-mcp-cli tool <name> [--param value]...`: Call a tool with parameters.

Use the `-server` flag to specify the server address (default: `localhost:8080`).

## **Security & Remote Access**

**Important:** This tool provides shell execution capabilities. Do not expose
this service to the public internet without authentication.

### **Non-Root Execution**

It is highly recommended to run `simple-mcp` as a dedicated non-root user (e.g.,
`simple-mcp`). The provided `simple-mcp.service` is configured to run as this
user and uses Systemd's hardening features to limit the process's capabilities.

If your tools require specific privileges (like `iptables` or `dmesg`), you
should grant them via `AmbientCapabilities` in the service file rather than
running the entire server as root.

### **Restricting Network Access**

By default, simple-mcp binds to localhost:8080, allowing only local connections.

The Model Context Protocol (MCP) does not implement authentication itself. If you
need to use simple-mcp outside of the machine it is running on, it is
highly recommended to use a **standard reverse proxy** (like Nginx, Apache,
Caddy, or Traefik) to handle:

* **TLS/SSL Encryption:** To protect data in transit.
* **Authentication:** To restrict access to authorized users (e.g., Basic
    Auth, OAuth2, OIDC).

### **Example: Nginx with Basic Auth**

Comprehensive examples for various reverse proxies can be found in the
[`webauth-examples/`](./webauth-examples/) directory:

* **Nginx:** [Basic Auth](./webauth-examples/nginx-basic-auth.conf), [OAuth2/OIDC](./webauth-examples/nginx-oauth2-proxy.conf)
* **Apache:** [Basic Auth](./webauth-examples/apache-basic-auth.conf), [OpenID Connect](./webauth-examples/apache-oidc.conf)
* **Caddy:** [Basic Auth & Forward Auth](./webauth-examples/Caddyfile)
* **Traefik:** [Docker Labels for Basic/Forward Auth](./webauth-examples/traefik-docker-compose.yaml)

### **Parameters and Shell Injection**

`simple-mcp` uses environment variables to pass parameters to shell commands.
This prevents direct command injection (e.g., passing `dummy; touch /tmp/evil`
as a parameter will not execute the second command).

However, users must still be careful when designing tool commands:

* **Argument Splitting & Globbing:** By default, if you use `{{.param}}`
    without quotes, the shell will expand the variable and then perform word
    splitting and globbing. For example, if `param` is `hello world`, `echo {{.param}}`
    becomes `echo hello world` (two arguments). If `param` is `*`, it might
    expand to a list of files.
* **Use Double Quotes:** It is highly recommended to use double quotes around
    parameters: `"{{.param}}"`. This ensures the shell treats the parameter
    as a single string and prevents globbing.
* **Input Validation:** While direct execution is prevented, parameters are
    still passed to underlying programs. Ensure these programs handle untrusted
    input safely and don't have vulnerabilities that could be triggered by
    maliciously crafted arguments (e.g., "Bobby Tables" scenarios).
* **Avoid `eval`:** Do not use `eval` or similar constructs with parameters
    in your commands, as this would re-introduce shell injection risks.

## **Usage with mcphost**

This repository includes an example configuration for mcphost. To use it

1. Ensure ollama is running and you have the qwen3-coder:30b (or similar) model
   pulled.
2. Edit mcphost.yaml if you need to change the model name or provider URL.
3. Run mcphost: mcphost \--config mcphost.yaml

**Note:** The mcphost.yaml configuration references systemprompt.txt using a
relative path. You must run mcphost from the directory containing
systemprompt.txt (or update the yaml to provide the full path), otherwise
mcphost will fail to load the system prompt.

## **License**

This project is licensed under the MIT License \- see the
[LICENSE](https://www.google.com/search?q=LICENSE) file for details.

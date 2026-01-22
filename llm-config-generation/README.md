# **Simple-MCP Configuration Generator**

This directory contains resources to help you generate secure, production-ready configurations for simple-mcp using any Large Language Model (LLM) like Gemini, Claude, or local models (Llama, Qwen, etc.).

By uploading the simple-mcp source code and the system prompt provided here to an LLM, you can turn the model into a specialized "Security Architect" that understands the nuances of the server's security model.

## **Files**

* **system\_prompt\_mcp\_architect.md**: This is the core instruction set for the LLM. It defines the rules for generating secure YAML configurations and hardened Systemd service files. It specifically addresses:  
  * **Parameter Quoting:** Enforcing correct shell variable usage ("${\_MCP\_VAR\_...}") to prevent injection and globbing vulnerabilities.  
  * **Systemd Hardening:** Mandating sandboxing features like ProtectSystem, PrivateNetwork, and CapabilityBoundingSet.  
  * **Scratch Space:** Properly configuring the tmpDir for safe file operations.  
  * **Async Tasks:** Identifying long-running jobs that must be executed asynchronously.

## **Usage**

1. **Prepare the Context:**  
   * Gather the simple-mcp source code files (or just the critical ones like executor.go, config.go, and main.go).  
   * Copy the content of system\_prompt\_mcp\_architect.md.  
2. **Prompt the LLM:**  
   * Paste the system prompt into the LLM's chat interface.  
   * Upload the source code files so the LLM has perfect context of the implementation.  
   * Ask your specific question or describe your use case.

**Example User Prompt:**"I want to create an MCP server that allows an LLM to manage Docker containers (list, start, stop, and logs). It needs to run as root but should be as restricted as possible."

3. **Review the Output:**  
   The LLM will generate two files:  
   * simple-mcp.yaml: The configuration file defining the tools and resources.  
   * simple-mcp.service: A hardened Systemd unit file to run the server securely.  
4. **Deploy:**  
   * Place simple-mcp.yaml in /etc/simple-mcp/ (or your preferred config location).  
   * Place simple-mcp.service in /etc/systemd/system/.  
   * Reload systemd and start the service:  
     sudo systemctl daemon-reload  
     sudo systemctl enable \--now simple-mcp

## **Why use this?**

Writing secure configurations that bridge the gap between an LLM and a root-level shell is difficult. This prompt ensures that the generated configuration follows best practices by default, minimizing the risk of command injection or privilege escalation.
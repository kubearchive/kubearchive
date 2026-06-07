# IDE Debug Configuration

## VSCode

Create or update `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Attach to KubeArchive",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "",
      "port": 40000,
      "host": "127.0.0.1",
      "showLog": true,
      "trace": "log",
      "logOutput": "rpc"
    }
  ]
}
```

**Key settings explained:**
- `"request": "attach"` -- connects to a running Delve instance rather than starting a new debug session
- `"port": 40000` -- matches the debug port exposed by `debug-deploy.sh`
- `"remotePath": ""` -- left blank because `ko build` compiles in the container root directory; setting this incorrectly causes breakpoints to silently fail

**Workflow:**
1. Ensure the debug-deployed pod is running and port 40000 is forwarded
2. Set breakpoints in the source code
3. Run the "Attach to KubeArchive" debug configuration
4. Verify breakpoints appear solid (not hollow) in the gutter -- hollow means the debugger can't map the source
5. Generate traffic to trigger your breakpoints

**Troubleshooting:**
- If breakpoints are hollow, double-check `remotePath` is empty
- If connection refused, verify `kubectl port-forward` is running and includes port 40000
- Check container logs for Delve startup confirmation

More details: https://golangforall.com/en/post/go-docker-delve-remote-debug.html#visual-studio-code

## GoLand

1. Go to **Run > Edit Configurations**
2. Click **+** and select **Go Remote**
3. Configure:
   - **Host:** `localhost`
   - **Port:** `40000`
4. Click **OK**, set breakpoints, and run the configuration

More details: https://golangforall.com/en/post/go-docker-delve-remote-debug.html#goland-ide

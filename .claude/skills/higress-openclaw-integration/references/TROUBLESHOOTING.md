# Higress AI Gateway - Troubleshooting

Common issues and solutions for Higress AI Gateway deployment and operation.

## Container Issues

### Container fails to start

**Check Docker is running:**
```bash
docker info
```

**Check port availability:**
```bash
netstat -tlnp | grep 8080
```

**View container logs:**
```bash
docker logs higress-ai-gateway
```

### Gateway not responding

**Check container status:**
```bash
docker ps -a
```

**Verify port mapping:**
```bash
docker port higress-ai-gateway
```

**Test locally:**
```bash
curl http://localhost:8080/v1/models
```

## File System Issues

### "too many open files" error from API server

**Symptom:**
```
panic: unable to create REST storage for a resource due to too many open files, will die
```
or
```
command failed err="failed to create shared file watcher: too many open files"
```

**Root Cause:**

The system's `fs.inotify.max_user_instances` limit is too low. This commonly occurs on systems with many Docker containers, as each container can consume inotify instances.

**Check current limit:**
```bash
cat /proc/sys/fs/inotify/max_user_instances
```

Default is often 128, which is insufficient when running multiple containers.

**Solution:**

Increase the inotify instance limit to 8192:

```bash
# Temporarily (until next reboot)
sudo sysctl -w fs.inotify.max_user_instances=8192

# Permanently (survives reboots)
echo "fs.inotify.max_user_instances = 8192" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

**Verify:**
```bash
cat /proc/sys/fs/inotify/max_user_instances
# Should output: 8192
```

**Restart the container:**
```bash
docker restart higress-ai-gateway
```

**Additional inotify tunables** (if still experiencing issues):
```bash
# Increase max watches per user
sudo sysctl -w fs.inotify.max_user_watches=524288

# Increase max queued events
sudo sysctl -w fs.inotify.max_queued_events=32768
```

To make these permanent as well:
```bash
echo "fs.inotify.max_user_watches = 524288" | sudo tee -a /etc/sysctl.conf
echo "fs.inotify.max_queued_events = 32768" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

## Plugin Issues

### Plugin not recognized

**Verify plugin installation:**

For Clawdbot:
```bash
ls -la ~/.clawdbot/extensions/higress-ai-gateway
```

For OpenClaw:
```bash
ls -la ~/.openclaw/extensions/higress-ai-gateway
```

**Check package.json:**

Ensure `package.json` contains the correct extension field:
- Clawdbot: `"clawdbot.extensions"`
- OpenClaw: `"openclaw.extensions"`

**Restart the runtime:**
```bash
# Restart Clawdbot gateway
clawdbot gateway restart

# Or OpenClaw gateway
openclaw gateway restart
```

## Routing Issues

### Auto-routing not working

**Confirm model is in list:**
```bash
# Check if higress/auto is available
clawdbot models list | grep "higress/auto"
```

**Check routing rules exist:**
```bash
./get-ai-gateway.sh route list
```

**Verify default model is configured:**
```bash
./get-ai-gateway.sh config list
```

**Check gateway logs:**
```bash
docker logs higress-ai-gateway | grep -i routing
```

**View access logs:**
```bash
tail -f ./higress/logs/access.log
```

## Configuration Issues

### Timezone detection fails

**Manually check timezone:**
```bash
timedatectl show --property=Timezone --value
```

**Or check timezone file:**
```bash
cat /etc/timezone
```

**Fallback behavior:**
- If detection fails, defaults to Hangzhou mirror
- Manual override: Set `IMAGE_REPO` environment variable

**Manual repository selection:**
```bash
# For China/Asia
IMAGE_REPO="higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one"

# For Southeast Asia
IMAGE_REPO="higress-registry.ap-southeast-7.cr.aliyuncs.com/higress/all-in-one"

# For North America
IMAGE_REPO="higress-registry.us-west-1.cr.aliyuncs.com/higress/all-in-one"

# Use in deployment
IMAGE_REPO="$IMAGE_REPO" ./get-ai-gateway.sh start --non-interactive ...
```

## Performance Issues

### Slow image downloads

**Check selected repository:**
```bash
echo $IMAGE_REPO
```

**Manually select closest mirror:**

See [Configuration Issues → Timezone detection fails](#timezone-detection-fails) for manual repository selection.

### High memory usage

**Check container stats:**
```bash
docker stats higress-ai-gateway
```

**View resource limits:**
```bash
docker inspect higress-ai-gateway | grep -A 10 "HostConfig"
```

**Set memory limits:**
```bash
# Stop container
./get-ai-gateway.sh stop

# Manually restart with limits
docker run -d \
  --name higress-ai-gateway \
  --memory="4g" \
  --memory-swap="4g" \
  ...
```

## Log Analysis

### Access logs location

```bash
# Default location
./higress/logs/access.log

# View real-time logs
tail -f ./higress/logs/access.log
```

### Container logs

```bash
# View all logs
docker logs higress-ai-gateway

# Follow logs
docker logs -f higress-ai-gateway

# Last 100 lines
docker logs --tail 100 higress-ai-gateway

# With timestamps
docker logs -t higress-ai-gateway
```

## Network Issues

### Cannot connect to gateway

**Verify container is running:**
```bash
docker ps | grep higress-ai-gateway
```

**Check port bindings:**
```bash
docker port higress-ai-gateway
```

**Test from inside container:**
```bash
docker exec higress-ai-gateway curl localhost:8080/v1/models
```

**Check firewall rules:**
```bash
# Check if port is accessible
sudo ufw status | grep 8080

# Allow port (if needed)
sudo ufw allow 8080/tcp
```

### DNS resolution issues

**Test from container:**
```bash
docker exec higress-ai-gateway ping -c 3 api.openai.com
```

**Check DNS settings:**
```bash
docker exec higress-ai-gateway cat /etc/resolv.conf
```

## Getting Help

If you're still experiencing issues:

1. **Collect logs:**
   ```bash
   docker logs higress-ai-gateway > gateway.log 2>&1
   cat ./higress/logs/access.log > access.log
   ```

2. **Check system info:**
   ```bash
   docker version
   docker info
   uname -a
   cat /proc/sys/fs/inotify/max_user_instances
   ```

3. **Report issue:**
   - Repository: https://github.com/higress-group/higress-standalone
   - Include: logs, system info, deployment command used

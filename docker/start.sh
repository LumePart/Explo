#!/bin/sh

PUID="${PUID:-0}"
PGID="${PGID:-0}"


# Update yt-dlp on startup
pip install --disable-pip-version-check --root-user-action=ignore --no-cache-dir --upgrade  yt-dlp
# Clear crontabs
crontab -r

# Update yt-dlp periodically
YTDLP_UPDATE="${YTDLP_UPDATE:-59 23 * * *}"
echo "$YTDLP_UPDATE echo 'Updating yt-dlp...' && pip install --disable-pip-version-check --root-user-action=ignore --no-cache-dir --upgrade yt-dlp >> /proc/1/fd/1 2>&1" >> /etc/crontabs/root
echo "[setup] Registered yt-dlp update job"
echo "        Schedule: $YTDLP_UPDATE"

if [ "$PUID" != "0" ] || [ "$PGID" != "0" ]; then
    groupmod -o -g "$PGID" explo
    usermod -o -u "$PUID" explo
    RUN_USER="explo"
    RUNNER="su-exec explo"
else
    echo "[setup] WARN: running as root. Consider defining PUID & PGID in docker-compose to run as a non-root user"
    RUN_USER="root"
    RUNNER=""
fi

mkdir -p /opt/explo
if [ "$RUN_USER" != "root" ]; then 
  chown -R explo:explo /opt/explo
fi

echo "[setup] Starting web UI..."
# If user incorectly mounts the config path as a directory, we'll try to automatically append it to .env inside it instead of failing.
WEB_ENV_PATH="${WEB_ENV_PATH:-/opt/explo/.env}"
if [ -d "$WEB_ENV_PATH" ]; then
    WEB_ENV_PATH="$WEB_ENV_PATH/.env"
    echo "[setup] Config path is a directory, using $WEB_ENV_PATH"
fi
WEB_UI=true WEB_ENV_PATH="$WEB_ENV_PATH" WEB_ADDR="${WEB_ADDR:-:7288}" $RUNNER ./explo --config "$WEB_ENV_PATH" &
echo "[setup] Web UI available at http://localhost:${WEB_ADDR##*:}"

echo "[setup] Initializing cron jobs..."

# Load *_SCHEDULE and *_FLAGS from .env if not already set in the environment.
# This allows the web UI to configure schedules by writing to the .env file.
_cfg="${WEB_ENV_PATH:-/opt/explo/.env}"
if [ -f "$_cfg" ]; then
  while IFS= read -r _line; do
    case "$_line" in \#*|'') continue ;; esac
    _key="${_line%%=*}"
    case "$_key" in
      *_SCHEDULE|*_FLAGS)
        if [ -z "$(printenv "$_key" 2>/dev/null)" ]; then
          export "$_key=${_line#*=}"
        fi
        ;;
    esac
  done < "$_cfg"
fi


# $CRON_SHCEDULE was deprecated in v0.11.0, keeping this block for backwards compatibility
if [ -n "$CRON_SCHEDULE" ]; then
    echo "$CRON_SCHEDULE cd /opt/explo && $RUNNER ./explo --config \"$_cfg\" >> /proc/1/fd/1 2>&1" >> /etc/crontabs/root
    chmod 600 /etc/crontabs/root
    echo "[setup] Registered single CRON_SCHEDULE job: $CRON_SCHEDULE"
    crond -f -l 2
fi

# Loop over all *_SCHEDULE environment variables
for var in $(env | grep "_SCHEDULE=" | cut -d= -f1); do
  job="${var%_SCHEDULE}"                     # Job name (e.g WEEKLY_EXPLORATION)
  schedule="$(printenv "$var")"              # Cron schedule
  flags_var="${job}_FLAGS"
  flags="$(printenv "$flags_var")"           # e.g. --playlist weekly-exploration

  if [ -z "$schedule" ]; then
    echo "[setup] Skipping $job: schedule is empty"
    continue
  fi

  # Default: just run explo if flags are empty
  cmd="cd /opt/explo && $RUNNER ./explo --config \"$_cfg\" $flags >> /proc/1/fd/1 2>&1"

  echo "$schedule $cmd" >> /etc/crontabs/root
  echo "[setup] Registered job: $job"
  echo "        Schedule: $schedule"
  echo "        Command : ./explo --config $_cfg $flags"
done

chmod 600 /etc/crontabs/root

echo "[setup] Starting cron..."

if [ "$EXECUTE_ON_START" = "true" ]; then
    echo "[setup] Executing startup task..."  
    cd /opt/explo && $RUNNER ./explo --config "$_cfg" $START_FLAGS
    
fi
crond -f -l 2

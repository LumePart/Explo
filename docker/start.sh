#!/bin/sh
echo "[setup] Initializing cron jobs..."


# $CRON_SHCEDULE was deprecated in v0.11.0, keeping this block for backwards compatibility
if [ -n "$CRON_SCHEDULE" ]; then
    echo "$CRON_SCHEDULE apk add --upgrade yt-dlp && cd /opt/explo && ./explo >> /proc/1/fd/1 2>&1" > /etc/crontabs/root
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
  cmd="apk add --upgrade yt-dlp && cd /opt/explo && ./explo $flags >> /proc/1/fd/1 2>&1"

  echo "$schedule $cmd" >> /etc/crontabs/root
  echo "[setup] Registered job: $job"
  echo "        Schedule: $schedule"
  echo "        Command : ./explo $flags"
done

chmod 600 /etc/crontabs/root

echo "[setup] Starting cron..."
crond -f -l 2
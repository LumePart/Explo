#!/bin/sh

# Handle PUID/PGID
if [ "$PUID" != "0" ] && [ "$PGID" != "0" ]; then
    echo "[setup] Setting up user with PUID=$PUID and PGID=$PGID"

    # Create group if it doesn't exist
    if ! getent group explo > /dev/null 2>&1; then
        groupadd -g "$PGID" explo
    fi

    # Create user if it doesn't exist
    if ! getent passwd explo > /dev/null 2>&1; then
        useradd -u "$PUID" -g "$PGID" -d /opt/explo -s /bin/sh explo
    fi

    # Ensure explo user owns the working directory and data directory
    chown -R explo:explo /opt/explo
    [ -d /data ] && chown -R explo:explo /data

    # If running as non-root, exec as the explo user
    if [ "$(id -u)" = "0" ]; then
        exec su-exec explo "$0" "$@"
    fi
fi

echo "[setup] Initializing cron jobs..."

# Determine which user to run cron jobs as
CRON_USER="root"
if [ "$PUID" != "0" ] && [ "$PGID" != "0" ]; then
    CRON_USER="explo"
    # Create crontab directory for explo user if it doesn't exist
    mkdir -p /var/spool/cron/crontabs
    touch "/var/spool/cron/crontabs/$CRON_USER"
    chown "$CRON_USER:$CRON_USER" "/var/spool/cron/crontabs/$CRON_USER"
fi

if [ -n "$CRON_SCHEDULE" ]; then
    cmd="apk add --upgrade yt-dlp && cd /opt/explo && ./explo >> /proc/1/fd/1 2>&1"
    echo "$CRON_SCHEDULE $cmd" > "/var/spool/cron/crontabs/$CRON_USER"
    chmod 600 "/var/spool/cron/crontabs/$CRON_USER"
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

  echo "$schedule $cmd" >> "/var/spool/cron/crontabs/$CRON_USER"
  echo "[setup] Registered job: $job"
  echo "        Schedule: $schedule"
  echo "        Command : ./explo $flags"
done

chmod 600 "/var/spool/cron/crontabs/$CRON_USER"

echo "[setup] Starting cron..."
crond -f -l 2
# Cron schedule is set by compose or build files
echo "$CRON_SCHEDULE cd /opt/explo && ./explo >> /proc/1/fd/1 2>&1" > /etc/crontabs/root

crond -f -l 2
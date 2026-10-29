#!/bin/sh
# Asserts start.sh passes --user all at exactly the three cron/start explo
# invocations and never to the web-server launch. Run: sh docker/start-user-all_test.sh
script="$(dirname "$0")/start.sh"
fail=0

n=$(grep -c '\./explo .*--user all' "$script")
[ "$n" -eq 4 ] && echo "ok: 3 invocations + 1 informational line" || { echo "FAIL: expected 4 matching lines, got $n"; fail=1; }

for pat in 'explo-run.sh legacy .*--user all >>' 'explo-run.sh \\"\$job\\" .*\$flags --user all >>' '\$START_FLAGS --user all'; do
    grep -q "$pat" "$script" && echo "ok: $pat" || { echo "FAIL: missing $pat"; fail=1; }
done

if grep '^WEB_UI=true' "$script" | grep -q -- '--user'; then
    echo "FAIL: web-server line has --user"; fail=1
else
    echo "ok: web-server line untouched"
fi
exit $fail

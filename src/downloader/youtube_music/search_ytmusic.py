#!/usr/bin/env python3
import sys
import json
from ytmusicapi import YTMusic


# Simple wrapper to search YouTube Music and return results as JSON.
def main():
    if len(sys.argv) < 2:
        print("Usage: search_cli.py <query> [limit]", file=sys.stderr)
        sys.exit(1)

    query = sys.argv[1]
    limit = int(sys.argv[2]) if len(sys.argv) > 2 else 1

    yt = YTMusic()
    results = yt.search(query, limit=limit)

    # Print JSON to stdout for Go to parse
    print(json.dumps(results))


if __name__ == "__main__":
    main()

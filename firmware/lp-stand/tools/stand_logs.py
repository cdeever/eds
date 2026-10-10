#!/usr/bin/env python3
"""
Read the stands' events back from the EdS tenant's log store.

A stand publishes its key events to its MQTT log topic, the substrate's bridge
carries them into the tenant's device log partition, and this reads that
partition. The endpoint and read token come from the tenant's Terraform
outputs (infra/deevnet-tenant-eds), so nothing is typed or hardcoded.

  stand_logs.py                        the last hour
  stand_logs.py --since 1d             the last day
  stand_logs.py -f                     keep printing new events as they arrive
  stand_logs.py 'event:scene.*'        only events matching a LogsQL filter
  stand_logs.py 'device:lp-stand-01'   only one stand's
"""

import argparse
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

from provision_tenant import DEFAULT_TENANT_DIR, read_outputs

DEVICE_PARTITION = 2  # (index, 2) holds a tenant's device logs
FOLLOW_INTERVAL_SEC = 3
# A following query looks back this far, so a line the store had not yet
# flushed when it was last asked is not lost; duplicates are dropped below.
FOLLOW_OVERLAP = '30s'


def query(store, ctx, logsql, limit):
    data = urllib.parse.urlencode({'query': logsql, 'limit': limit}).encode()
    request = urllib.request.Request(
        store['endpoint'].rstrip('/') + '/select/logsql/query', data=data,
        headers={
            'Authorization': 'Bearer ' + store['read_token'],
            store['select_header']: f"{store['account_id']}-{DEVICE_PARTITION}",
        })
    with urllib.request.urlopen(request, context=ctx, timeout=15) as response:
        lines = [json.loads(line) for line in response.read().decode().splitlines() if line.strip()]
    # A stand has no clock, so events it queued before it connected arrive
    # together and are dated on receipt, within the same second and not in the
    # order they happened. Within a second, the stand's uptime orders them.
    return sorted(lines, key=lambda line: (line.get('_time', '')[:19], uptime_ms(line)))


def uptime_ms(line):
    try:
        return int(line.get('uptime_ms', 0))
    except (TypeError, ValueError):
        return 0


def render(line):
    when = line.get('_time', '')[:23].replace('T', ' ')
    known = {'_time', '_msg', '_stream', '_stream_id', 'tenant', 'device', 'topic', 'level', 'event', 'msg'}
    extra = ' '.join(f"{k}={v}" for k, v in sorted(line.items()) if k not in known)
    return (f"{when}  {line.get('device', '?'):<12} {line.get('level', '-'):<5} "
            f"{line.get('event', '-'):<18} {line.get('_msg', '')}" + (f"  [{extra}]" if extra else ''))


def main():
    parser = argparse.ArgumentParser(description="Read the stands' events from the EdS tenant's log store")
    parser.add_argument('filter', nargs='?', default='*',
                        help="LogsQL filter, e.g. 'event:scene.*' or 'level:error' (default: everything)")
    parser.add_argument('--since', default='1h', help='How far back to read, e.g. 15m, 6h, 2d (default: 1h)')
    parser.add_argument('-f', '--follow', action='store_true', help='Keep printing new events as they arrive')
    parser.add_argument('-n', '--limit', type=int, default=1000, help='Most events to read at once (default: 1000)')
    parser.add_argument('--tenant-dir', default=DEFAULT_TENANT_DIR,
                        help='Tenant Terraform directory (default: infra/deevnet-tenant-eds)')
    parser.add_argument('--outputs-json', default=None, metavar='FILE',
                        help="Read `terraform output -json` from FILE ('-' for stdin) instead of running terraform")
    args = parser.parse_args()

    try:
        outputs = read_outputs(args.tenant_dir, args.outputs_json)
        store = outputs['log_store']
        ca = os.path.join(args.tenant_dir, outputs['broker']['ca'])
        ctx = ssl.create_default_context(cafile=ca)
    except (RuntimeError, OSError, KeyError, ValueError) as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    seen = set()
    window = args.since
    try:
        while True:
            for line in query(store, ctx, f"_time:{window} ({args.filter})", args.limit):
                key = (line.get('_time'), line.get('_stream_id'), line.get('_msg'))
                if key not in seen:
                    seen.add(key)
                    print(render(line), flush=True)
            if not args.follow:
                return 0
            window = FOLLOW_OVERLAP
            time.sleep(FOLLOW_INTERVAL_SEC)
    except urllib.error.HTTPError as e:
        print(f"Error: the log store answered {e.code}: {e.read().decode()[:200]}", file=sys.stderr)
        return 1
    except (urllib.error.URLError, OSError) as e:
        print(f"Error: could not reach the log store: {e}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        return 0


if __name__ == '__main__':
    sys.exit(main())

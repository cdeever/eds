#!/usr/bin/env python3
"""
Provision an LP stand with what the EdS tenant issued for it.

Writes the stand's `creds` partition: the WiFi key it associates with, the
broker it dials, its own broker account, and the CA the broker's certificate
is verified against.

Every value is read from the tenant's Terraform outputs
(infra/deevnet-tenant-eds), so nothing is typed, pasted or left in shell
history. `make wifi-config` remains for a stand that is not on the substrate.

Only the creds partition is written. The firmware and the WiFi driver's own
NVS are untouched, so a stand can be reprovisioned without being rebuilt.
"""

import argparse
import csv
import json
import os
import re
import subprocess
import sys
import tempfile

PROJECT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REPO_ROOT = os.path.dirname(os.path.dirname(PROJECT_DIR))
DEFAULT_TENANT_DIR = os.path.join(REPO_ROOT, 'infra', 'deevnet-tenant-eds')

# Must match main/config/lp_credentials.h. A mismatch does not fail loudly: the
# firmware finds nothing and starts as an unprovisioned stand, with no keys.
NAMESPACE = 'lp-stand'
PARTITION = 'creds'

# What the firmware's buffers hold (lp_credentials.c), terminator excluded.
LIMITS = {
    'wifi ssid': 32,
    'wifi key': 63,
    'broker uri': 159,
    'broker username': 64,
    'broker password': 64,
}
MAX_CA_BYTES = 3999  # NVS holds a string of at most 4000 bytes


def backend_env(tenant_dir):
    """The environment plus the state store's credentials from .backend.env, as the Makefile does"""
    env = dict(os.environ)
    path = os.path.join(tenant_dir, '.backend.env')
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                name, sep, value = line.strip().partition('=')
                if sep and not name.startswith('#'):
                    env[name] = value
    return env


def read_outputs(tenant_dir, outputs_json=None):
    """Return the tenant's Terraform outputs as {name: value}"""
    if outputs_json:
        with (sys.stdin if outputs_json == '-' else open(outputs_json)) as f:
            raw = json.load(f)
    else:
        result = subprocess.run(['terraform', 'output', '-json'], cwd=tenant_dir,
                                capture_output=True, text=True, env=backend_env(tenant_dir))
        if result.returncode != 0:
            raise RuntimeError(f"terraform output failed in {tenant_dir}:\n{result.stderr.strip()}")
        raw = json.loads(result.stdout)

    if not raw:
        raise RuntimeError("the tenant has no outputs - has it been applied, and is the state reachable?")
    return {name: entry['value'] for name, entry in raw.items()}


def compiled_identity():
    """The stand id and topic prefix this checkout builds with: sdkconfig, else the Kconfig defaults"""
    identity = {'CONFIG_LP_STAND_ID': 'lp-stand-01', 'CONFIG_LP_TOPIC_PREFIX': 'eds'}
    path = os.path.join(PROJECT_DIR, 'sdkconfig')
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                match = re.match(r'(CONFIG_LP_STAND_ID|CONFIG_LP_TOPIC_PREFIX)="(.*)"\s*$', line)
                if match:
                    identity[match.group(1)] = match.group(2)
    return identity['CONFIG_LP_STAND_ID'], identity['CONFIG_LP_TOPIC_PREFIX']


def check_topics(account, stand, prefix):
    """
    The firmware's topics are compiled in; the broker's are whatever was granted.

    Worth checking here because the broker does not report a mismatch: a publish
    to a topic that was not granted is dropped, and a refused subscription looks
    like a quiet one. The stand would connect, and nothing would arrive.
    """
    wanted = {
        'publish': [f'{prefix}/lightstand/{stand}/status',
                    f'{prefix}/lightstand/{stand}/state',
                    f'{prefix}/log/{stand}'],
        'subscribe': [f'{prefix}/lightstand/{stand}/scene'],
    }
    missing = [f'{verb} {topic}' for verb, topics in wanted.items()
               for topic in topics if topic not in account.get(verb, [])]
    if missing:
        raise RuntimeError(
            "the stand's broker account was not granted what this firmware uses:\n  "
            + "\n  ".join(missing)
            + f"\nGranted: publish {account.get('publish')}, subscribe {account.get('subscribe')}")
    return wanted


def build_settings(outputs, tenant_dir, stand, prefix, ca_path):
    """Collect and validate everything that goes into the creds partition"""
    account_output = stand.replace('-', '_') + '_broker'
    for name in ('device_wifi', 'broker', account_output):
        if name not in outputs:
            raise RuntimeError(f"tenant output '{name}' is missing")

    wifi = outputs['device_wifi']
    broker = outputs['broker']
    account = outputs[account_output]
    topics = check_topics(account, stand, prefix)

    ca_path = ca_path or os.path.join(tenant_dir, broker['ca'])
    if not os.path.exists(ca_path):
        raise RuntimeError(f"broker CA not found: {ca_path}")
    with open(ca_path) as f:
        ca_pem = f.read()
    if 'BEGIN CERTIFICATE' not in ca_pem:
        raise RuntimeError(f"{ca_path} is not a PEM certificate")
    if len(ca_pem.encode()) > MAX_CA_BYTES:
        raise RuntimeError(f"{ca_path} is {len(ca_pem.encode())} bytes; NVS holds at most {MAX_CA_BYTES}")

    settings = {
        'wifi ssid': wifi['ssid'],
        'wifi key': wifi['psk'],
        'broker uri': f"mqtts://{broker['host']}:{int(broker['port'])}",
        'broker username': account['username'],
        'broker password': account['password'],
        'ca path': os.path.abspath(ca_path),
        'log topic': topics['publish'][2],
    }

    for name, limit in LIMITS.items():
        length = len(settings[name].encode())
        if length == 0:
            raise RuntimeError(f"{name} is empty")
        if length > limit:
            raise RuntimeError(f"{name} is {length} bytes; the firmware holds at most {limit}")

    return settings


def creds_partition():
    """(offset, size) of the creds partition, from partitions.csv - the one place the layout is written"""
    with open(os.path.join(PROJECT_DIR, 'partitions.csv')) as f:
        for line in f:
            fields = [field.strip() for field in line.split('#')[0].split(',')]
            if len(fields) >= 5 and fields[0] == PARTITION:
                return fields[3], fields[4]
    raise RuntimeError(f"partitions.csv has no {PARTITION} partition")


def write_nvs_csv(settings, path):
    rows = [
        ('key', 'type', 'encoding', 'value'),
        (NAMESPACE, 'namespace', '', ''),
        ('wifi_ssid', 'data', 'string', settings['wifi ssid']),
        ('wifi_pass', 'data', 'string', settings['wifi key']),
        ('mqtt_uri', 'data', 'string', settings['broker uri']),
        ('mqtt_user', 'data', 'string', settings['broker username']),
        ('mqtt_pass', 'data', 'string', settings['broker password']),
        ('mqtt_ca', 'file', 'string', settings['ca path']),
    ]
    with open(path, 'w', newline='') as f:
        csv.writer(f).writerows(rows)


def generate_nvs_image(csv_path, bin_path, size):
    idf_path = os.environ.get('IDF_PATH')
    if not idf_path:
        raise RuntimeError("IDF_PATH is not set. Activate ESP-IDF first.")

    generator = os.path.join(idf_path, 'components', 'nvs_flash',
                             'nvs_partition_generator', 'nvs_partition_gen.py')
    if not os.path.exists(generator):
        raise RuntimeError(f"cannot find {generator}")

    result = subprocess.run([sys.executable, generator, 'generate', csv_path, bin_path, size],
                            capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"generating the NVS image failed:\n{result.stderr.strip() or result.stdout.strip()}")


def main():
    parser = argparse.ArgumentParser(
        description="Provision an LP stand with the EdS tenant's WiFi key, broker account and CA")
    parser.add_argument('-p', '--port', default=None, help='Serial port the stand is on')
    parser.add_argument('--stand', default=None,
                        help='Stand to provision (default: the id this checkout builds with)')
    parser.add_argument('--tenant-dir', default=DEFAULT_TENANT_DIR,
                        help='Tenant Terraform directory (default: infra/deevnet-tenant-eds)')
    parser.add_argument('--outputs-json', default=None, metavar='FILE',
                        help="Read `terraform output -json` from FILE ('-' for stdin) instead of running terraform")
    parser.add_argument('--ca', default=None,
                        help="Broker CA in PEM (default: the file the tenant's broker output names)")
    parser.add_argument('--dry-run', action='store_true',
                        help='Build the image and stop, without touching the stand')
    args = parser.parse_args()

    try:
        stand, prefix = compiled_identity()
        stand = args.stand or stand
        settings = build_settings(read_outputs(args.tenant_dir, args.outputs_json),
                                  args.tenant_dir, stand, prefix, args.ca)
        offset, size = creds_partition()
    except (RuntimeError, OSError, KeyError, ValueError) as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    print(f"Provisioning {stand} from the tenant's outputs (secrets not shown):")
    print(f"  WiFi SSID:  {settings['wifi ssid']}")
    print(f"  Broker:     {settings['broker uri']}")
    print(f"  Account:    {settings['broker username']}")
    print(f"  Broker CA:  {settings['ca path']}")
    print(f"  Events on:  {settings['log topic']}")

    # The CSV holds the WiFi key and the broker password: private, outside the
    # repository, and gone as soon as the image is built.
    with tempfile.TemporaryDirectory() as tmp:
        csv_path = os.path.join(tmp, 'creds.csv')
        bin_path = os.path.join(tmp, 'creds.bin')

        try:
            write_nvs_csv(settings, csv_path)
            generate_nvs_image(csv_path, bin_path, size)
        except (RuntimeError, OSError) as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1

        if args.dry_run:
            print(f"Dry run: a {size} image was built for {offset}; the stand was not touched")
            return 0
        if not args.port:
            print("Error: no serial port. Pass -p, or PORT= to make.", file=sys.stderr)
            return 1

        result = subprocess.run([sys.executable, '-m', 'esptool', '-p', args.port,
                                 'write-flash', offset, bin_path],
                                capture_output=True, text=True)
        if result.returncode != 0:
            print(f"Error flashing the stand: {result.stderr.strip() or result.stdout.strip()}", file=sys.stderr)
            return 1

    print(f"Provisioned {size} bytes at {offset}; the stand restarts with them.")
    return 0


if __name__ == '__main__':
    sys.exit(main())

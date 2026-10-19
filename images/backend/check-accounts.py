#!/usr/bin/env python3
"""
Are the backend Pi's broker accounts the tenant's?

The accounts are written down twice: as Terraform for the tenant
(infra/deevnet-tenant-eds/main.tf) and as a list for a Pi that is its own
broker (images/backend/files/accounts.conf, ADR-0033). A broker says nothing
about a topic it was not told to allow, so a difference between the two is a
Pi on which something is quietly not heard.

Reads both and exits non-zero, naming each difference, when they disagree.
"""

import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
TERRAFORM = os.path.join(REPO, 'infra', 'deevnet-tenant-eds', 'main.tf')
LIST = os.path.join(HERE, 'files', 'accounts.conf')


def strings(block, key):
    """The quoted strings in `key = [ ... ]`, comments and all removed"""
    match = re.search(r'^\s*%s\s*=\s*\[(.*?)\]' % key, block, re.S | re.M)
    if not match:
        return []
    body = re.sub(r'#[^\n]*', '', match.group(1))
    return re.findall(r'"([^"]*)"', body)


def from_terraform(path):
    with open(path) as f:
        text = f.read()
    accounts = {}
    for match in re.finditer(r'^resource "deevnet_iot_broker_account" "\w+" \{\n(.*?)^\}', text, re.S | re.M):
        block = match.group(1)
        name = re.search(r'^\s*name\s*=\s*"([^"]+)"', block, re.M).group(1)
        # A device is named by reference to its resource, whose name is the
        # account's own in every case so far. What matters is that there is one.
        device = bool(re.search(r'^\s*device\s*=', block, re.M))
        accounts[name] = (device, strings(block, 'publish'), strings(block, 'subscribe'))
    return accounts


def from_list(path):
    accounts = {}
    with open(path) as f:
        for number, line in enumerate(f, 1):
            line = line.split('#')[0].strip()
            if not line:
                continue
            fields = line.split()
            if len(fields) != 4:
                sys.exit('%s:%d: four fields are wanted, and there are %d' % (path, number, len(fields)))
            name, device, publish, subscribe = fields
            if device not in ('-', name):
                sys.exit("%s:%d: %s's device is %r; a device account is named for its device" % (path, number, name, device))
            patterns = lambda field: [] if field == '-' else field.split(',')
            accounts[name] = (device != '-', patterns(publish), patterns(subscribe))
    return accounts


def main():
    tenant, pi = from_terraform(TERRAFORM), from_list(LIST)
    if not tenant:
        sys.exit('no broker accounts were found in %s' % TERRAFORM)
    wrong = []
    for name in sorted(set(tenant) | set(pi)):
        if name not in pi:
            wrong.append('%s is in the Terraform and not in accounts.conf' % name)
        elif name not in tenant:
            wrong.append('%s is in accounts.conf and not in the Terraform' % name)
        else:
            for what, a, b in zip(('device', 'publish', 'subscribe'), tenant[name], pi[name]):
                if a != b:
                    wrong.append('%s: %s differs\n    tenant: %s\n    pi:     %s' % (name, what, a, b))
    if wrong:
        sys.exit('The backend Pi\'s accounts are not the tenant\'s:\n  ' + '\n  '.join(wrong))
    print('%d accounts, the same in both' % len(tenant))


if __name__ == '__main__':
    main()

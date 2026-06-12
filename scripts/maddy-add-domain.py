#!/usr/bin/env python3
"""Add domain to Maddy config - called from add-domain.sh"""
import re
import sys

domain = sys.argv[1]
conf_path = "/etc/maddy/maddy.conf"

with open(conf_path) as f:
    content = f.read()

if domain in content:
    print(f"  Domain {domain} already in Maddy config")
else:
    # Find local_domains line and append domain
    content = re.sub(
        r'(\$\(local_domains\) = .+)',
        lambda m: m.group(1) + ' ' + domain,
        content,
        count=1
    )
    with open(conf_path, 'w') as f:
        f.write(content)
    print(f"  Added {domain} to local_domains")

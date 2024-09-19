---
title: "MDNS"
date: 2019-03-03T16:39:46+01:00
draft: false
slug: mdns
dnsprovider:
  since:    "v4.16.0"
  code:     "mdns"
  url:      "https://mdns.nic.md/api/"
---

<!-- THIS DOCUMENTATION IS AUTO-GENERATED. PLEASE DO NOT EDIT. -->
<!-- providers/dns/mdns/mdns.toml -->
<!-- THIS DOCUMENTATION IS AUTO-GENERATED. PLEASE DO NOT EDIT. -->


Configuration for [MDNS](https://mdns.nic.md/api/).


<!--more-->

- Code: `mdns`
- Since: v4.16.0


Here is an example bash command using the MDNS provider:

```bash
MDNS_AUTH_EMAIL=username@example.com \
MDNS_AUTH_KEY=XXXXXXXXXXXXXX \
MDNS_BASE_URL=https://mdns.nic.md/api \
lego --email you@example.com --dns mdns --domains example.md run

MDNS_AUTH_FILE=.secret.example.md ./lego --email info@example.com --dns mdns --domains example.md --dns.disable-cp run
```




## Credentials

| Environment Variable Name | Description |
|-----------------------|-------------|
| `MDNS_AUTH_EMAIL` | Username email |
| `MDNS_AUTH_KEY` | Authorization key |
| `MDNS_BASE_URL` | Base API URL (ex: https://dns.nic.md/) |

The environment variable names can be suffixed by `_FILE` to reference a file instead of a value.
More information [here]({{% ref "dns#configuration-and-credentials" %}}).


## Additional Configuration

| Environment Variable Name | Description |
|--------------------------------|-------------|
| `MDNS_HTTP_TIMEOUT` | API request timeout |
| `MDNS_POLLING_INTERVAL` | Time between DNS propagation check |
| `MDNS_PROPAGATION_TIMEOUT` | Maximum waiting time for DNS propagation |
| `MDNS_TTL` | The TTL of the TXT record used for the DNS challenge |

The environment variable names can be suffixed by `_FILE` to reference a file instead of a value.
More information [here]({{% ref "dns#configuration-and-credentials" %}}).




## More information

- [API documentation](https://mdns.nic.md/api/doc)

<!-- THIS DOCUMENTATION IS AUTO-GENERATED. PLEASE DO NOT EDIT. -->
<!-- providers/dns/mdns/mdns.toml -->
<!-- THIS DOCUMENTATION IS AUTO-GENERATED. PLEASE DO NOT EDIT. -->

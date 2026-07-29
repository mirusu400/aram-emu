# Compatibility records

ARAM compatibility is tracked per file hash and profile, not by anecdotal
platform-wide labels.

Each record will contain:

```yaml
title: Example
sha256: 0000000000000000000000000000000000000000000000000000000000000000
format: eads
profile: samsung/skt/example
backend: unicorn
result: boots
aram_commit: 0000000
input_replay: traces/example.json
notes: Synthetic example only
```

Supported result levels are `recognized`, `loads`, `boots`, `menu`,
`playable`, and `complete`. A higher level requires reproducible evidence for
all lower levels.

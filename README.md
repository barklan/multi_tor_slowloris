# distributed_slowloris

Tested with 2 servers with 128 tor connections each (may have had overlapping exit nodes). Possibly diminishing returns with more connections due to limited number of tor exit nodes. Also probably damaging to the tor network itself.

Needs increased fd and npocs limits. tcp_reuse kernel parameter might benefit.

...

`/etc/security/limits.conf`

```
barklan soft nofile 16384
barklan hard nofile 32768
```

Running in containers is noop.

#!/bin/bash

initport="9060"
howmany=128

for ((i=0;i<howmany;i++));do
    sudo touch /etc/tor/torrc."$i"
    portinc=$((i * 10))
    port=$((initport + portinc))
    control=$((port + 1))
    # sudo bash -c "printf 'SocksPort $port \nControlPort $control\nDataDirectory /var/lib/tor$i\nExitNodes {ru},{de} StrictNodes 1\n' > /etc/tor/torrc.$i"
    sudo bash -c "printf 'SocksPort $port \nControlPort $control\nDataDirectory /var/lib/tor$i\n' > /etc/tor/torrc.$i"
done

torpids=""
for ((j=0;j<howmany;j++));do
    sudo tor -f /etc/tor/torrc."${j}" &
    torpids+=" $!"
    sleep 0.3
done

for p in $torpids; do
    wait "$p"
done

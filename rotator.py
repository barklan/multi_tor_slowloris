from stem import Signal
from stem.control import Controller
import os
import time


control_ports = []
initial = 9061

for i in range(64):
    control_ports.append(initial+i*10)

while True:
    time.sleep(300)
    for i in range(64):
        time.sleep(1)
        with Controller.from_port(port = control_ports[i]) as controller:
            controller.authenticate()
            controller.signal(Signal.NEWNYM)

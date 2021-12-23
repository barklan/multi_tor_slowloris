from stem import Signal
from stem.control import Controller
import os


control_port = int(os.getenv('CONTROL_PORT'))

with Controller.from_port(port = control_port) as controller:
    controller.authenticate()
    controller.signal(Signal.NEWNYM)

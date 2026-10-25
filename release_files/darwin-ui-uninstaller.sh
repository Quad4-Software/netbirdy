#!/bin/sh

export PATH=$PATH:/usr/local/bin

# check if netbirdy is installed
NB_BIN=$(which netbirdy)
if [ -z "$NB_BIN" ]
then
  exit 0
fi
# start netbirdy daemon service
echo "netbirdy daemon service still running. You can uninstall it by running: "
echo "sudo netbirdy service stop"
echo "sudo netbirdy service uninstall"

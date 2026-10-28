#!/bin/sh

export PATH=$PATH:/usr/local/bin:/opt/homebrew/bin

# check if netbirdy is installed
NB_BIN=$(which netbirdy)
if [ -n "$NB_BIN" ]
then
  echo "Stopping and uninstalling NetBirdy daemon"
  netbirdy service stop || true
  netbirdy service uninstall || true
fi

# check if netbirdy is installed
NB_BIN=$(which netbirdy)
if [ -z "$NB_BIN" ]
then
  echo "NetBirdy daemon is not installed. Please run: brew install quad4-software/tap/netbirdy"
  exit 1
fi
NB_UI_VERSION=$1
NB_VERSION=$(netbirdy version)
if [ "X-$NB_UI_VERSION" != "X-$NB_VERSION" ]
then
  echo "NetBirdy's daemon is running with a different version than the NetBirdy's UI:"
  echo "NetBirdy UI Version: $NB_UI_VERSION"
  echo "NetBirdy Daemon Version: $NB_VERSION"
  echo "Please run: brew install quad4-software/tap/netbirdy"
  echo "to update it"
fi

if [ -n "$NB_BIN" ]
then
  echo "Stopping NetBirdy daemon"
  osascript -e 'quit app "NetBirdy UI"' 2> /dev/null || true
  netbirdy service stop 2> /dev/null || true
fi

# start netbirdy daemon service
echo "Starting NetBirdy daemon"
netbirdy service install 2> /dev/null || true
netbirdy service start || true

# start app
open /Applications/NetBirdy\ UI.app

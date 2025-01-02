#!/bin/bash

FPP_INFO=/home/fpp/media/fpp-info.json
PSK=""

if [ "x${PSK}" = "x" ]; then
  echo "Set (but do not checkin) the WiFi PSK before using."
  exit 1
fi

declare -A DESCRIPTIONS=(
  [192.168.1.32]="K16 N Tunnel"
  [192.168.1.33]="K16 S Tunnel"
  [192.168.1.40]="K16 Water Tower"
  [192.168.1.41]="K16 Fence W of Station House"
  [192.168.1.42]="K16 Fence Center W"
  [192.168.1.43]="K16 Fence NW"
  [192.168.1.44]="K16 N Gate Arch and NE Fence"
  [192.168.1.45]="K16 Fence N of Restrooms"
  [192.168.1.46]="K16 Fence S of Restrooms"
  [192.168.1.47]="K16 Fence E of Central Arch"
  [192.168.1.48]="K16 Fence E of S Table Arches"
  [192.168.1.62]="K16 SW North Forest"
  [192.168.1.63]="K16 NW North Forest"
  [192.168.1.64]="K16 E North Forest"
  [192.168.1.170]="PiHat Gift Box, NW of Tree"
  [192.168.1.171]="PiHat Gift Box, NW of Tree"
  [192.168.1.172]="PiHat Gift Box, NW of Tree"
  [192.168.1.173]="PiHat Gift Box, NW of Tree"
  [192.168.1.175]="PiHat Gift Box, SW of Tree"
  [192.168.1.176]="PiHat Gift Box, SW of Tree"
  [192.168.1.182]="PiHat Mini Gift Box, SW of Tree"
  [192.168.1.150]="K16 Station Arch"
  [192.168.1.151]="K16 N - South Table Arches"
  [192.168.1.152]="K16 Center - South Table Arches"
  [192.168.1.153]="K16 S - South Table Arches"
  [192.168.1.155]="K64 GigaTree Controller on NE Fence"
  [192.168.1.202]="BBB Audio Box"
  [192.168.1.203]="BBB Audio Box"
  [192.168.1.204]="RPi - S of Restrooms Audio Box"
  [192.168.1.205]="BBB Audio Box"
  [192.168.1.206]="BBB Audio Box"
  [192.168.1.207]="BBB Audio Box"
)

declare -A WIFI_ADDR=(
  [192.168.1.170]="192.168.1.70"
  [192.168.1.171]="192.168.1.71"
  [192.168.1.172]="192.168.1.72"
  [192.168.1.173]="192.168.1.73"
  [192.168.1.175]="192.168.1.75"
  [192.168.1.176]="192.168.1.76"
  [192.168.1.182]="192.168.1.82"
)

for addr in "${!DESCRIPTIONS[@]}"; do
  descrip=${DESCRIPTIONS[${addr}]}
  wifi=${WIFI_ADDR[${addr}]}

  echo "Configuring ${addr}:"
  echo "  Description: \"${descrip}\""
  echo "  WiFi: ${wifi}"

  echo -n '  ... Set Host Description: '
  curl -X PUT -H 'Content-Type: application/json' -d "${DESCRIPTIONS[${addr}]}" "http://${addr}/api/settings/HostDescription"
  echo ''

  ## Key FPP settings
  echo -n '   ... Enable RSync: '
  curl -X PUT -H 'Content-Type: application/json' -d '1' "http://${addr}/api/settings/Service_rsync"
  echo ''
  echo -n '   ... Enable FPP MultiSync: '
  curl -X PUT -H 'Content-Type: application/json' -d '1' "http://${addr}/api/settings/MultiSyncEnabled"
  echo ''
  echo -n '   ... Set to FPP Remote mode: '
  curl -X PUT -H 'Content-Type: application/json' -d 'remote' "http://${addr}/api/settings/fppMode"
  echo ''
  echo -n '   ... Prioritize Sequence when there is also DDP inbound: '
  curl -X PUT -H 'Content-Type: application/json' -d 'Prioritize Sequence' "http://${addr}/api/settings/bridgeDataPriority"
  echo ''

  ## Wipe the strands when a sequence ends. Doesn't really do much, but want it consistant.
  echo -n '   ... Enable Blank between Sequences: '
  curl -X PUT -H 'Content-Type: application/json' -d '1' "http://${addr}/api/settings/blankBetweenSequences"
  echo ''

  ## Probably for WiFi setting
  echo -n '   ... Set locale to USA: '
  curl -X PUT -H 'Content-Type: application/json' -d 'USA' "http://${addr}/api/settings/Locale"
  echo ''

  ### Configure Ethernet
  eth_gateway="192.168.1.1"
  [ "x${wifi}" != "x" ] && eth_gateway=""
  echo -n '   ... Configure ethernet (eth0): '
  curl -X POST -H 'Content-Type: application/json' -d "{\"INTERFACE\":\"eth0\",\"PROTO\":\"static\",\"ADDRESS\":\"${addr}\",\"NETMASK\":\"255.255.255.0\",\"GATEWAY\":\"${eth_gateway}\",\"ROUTEMETRIC\":\"0\",\"DHCPSERVER\":false,\"DHCPOFFSET\":\"100\",\"DHCPPOOLSIZE\":\"50\",\"IPFORWARDING\":\"0\",\"Leases\":{}}" "http://${addr}/api/network/interface/eth0"
  echo ''

  if [ "x${wifi}" != "x" ]; then
    echo -n '   ... Configure WiFi (wlan0): '
    curl -X POST -H 'Content-Type: application/json' -d "{\"INTERFACE\":\"wlan0\",\"PROTO\":\"static\",\"ADDRESS\":\"${wifi}\",\"NETMASK\":\"255.255.255.0\",\"GATEWAY\":\"192.168.1.1\",\"ROUTEMETRIC\":\"0\",\"DHCPSERVER\":false,\"DHCPOFFSET\":\"100\",\"DHCPPOOLSIZE\":\"50\",\"IPFORWARDING\":\"0\",\"SSID\":\"KIR Lights\",\"PSK\":\"${PSK}\",\"HIDDEN\":false,\"WPA3\":false,\"BACKUPSSID\":\"\",\"BACKUPPSK\":\"\",\"BACKUPHIDDEN\":false,\"BACKUPWPA3\":false,\"Leases\":{}}" "http://${addr}/api/network/interface/wlan0"
    echo ''
  fi

  ## Configure DNS
  echo -n '   ... Configure DNS: '
  curl -X POST -H 'Content-Type: application/json' -d '{"DNS1":"8.8.8.8","DNS2":"8.8.4.4"}' "http://${addr}/api/network/dns"
  echo ''

  ## Configure Time
  echo -n '   ... Configure TimeZone: '
  curl -X PUT -H 'Content-Type: application/json' -d 'America/Los_Angeles' "http://${addr}/api/settings/TimeZone"
  echo ''
  echo -n '   ... Configure NTP Server: '
  curl -X PUT -H 'Content-Type: application/json' -d 'time.google.com' "http://${addr}/api/settings/ntpServer"
  echo ''

  # Misc stuff.
  echo -n '   ... Set UI to advanced mode: '
  curl -X PUT -H 'Content-Type: application/json' -d '1' "http://${addr}/api/settings/uiLevel"
  echo ''
  # ... uhhh, I don't really know why I care to populate this correctly, it's probably only used for usage reports.
  echo -n '   ... Set Latitude: '
  curl -X PUT -H 'Content-Type: application/json' -d '47.6784' "http://${addr}/api/settings/Latitude"
  echo ''
  echo -n '   ... Set Longitude: '
  curl -X PUT -H 'Content-Type: application/json' -d '-122.1857' "http://${addr}/api/settings/Longitude"
  echo ''
  echo -n '   ... Disable stats publication: '
  curl -X PUT -H 'Content-Type: application/json' -d 'Disabled' "http://${addr}/api/settings/statsPublish"
  echo ''
  echo -n '   ... Disable sharing crash data: '
  curl -X PUT -H 'Content-Type: application/json' -d '0' "http://${addr}/api/settings/ShareCrashData"
  echo ''

  ## Restart network
  echo -n '   ... Restart the network: '
  curl -X POST -H 'Content-Type: application/json' "http://${addr}/api/network/interface/eth0/apply"
  echo ""

  echo ""
done

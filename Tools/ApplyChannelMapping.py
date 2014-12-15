
# Copyright 2014 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
import sys
import re
import csv

from xml.dom.minidom import parse
from collections import namedtuple

GroundRing = namedtuple("GroundRing", ["zone", "ring"])

def LoadDmxMappings(file_name):
	dmxDict = {}
	with open(file_name, 'rb') as csvfile:
		dmxCsv = csv.DictReader(csvfile)
		for row in dmxCsv:
			dmxDict[row['Description']] = row['DMX Channel']
	return dmxDict

# <channel name="Zone 1 - Sparkle Color (B)" color="16711680" deviceType="DMX Universe" circuit="40" network="1" savedIndex="144"/>
def ProcessFile(dmxDict, file_name):
	print file_name
	dom = parse(file_name)
	# sequence = dom.getElementsByTagName('sequence')[0]
	channels = dom.getElementsByTagName('channels')[0]

	for channel in channels.getElementsByTagName('channel'):
		channel_name = channel.getAttribute('name')
		if channel_name:
			dmxId = dmxDict.pop(channel_name, None)
			if dmxId == None:
				print "No DMX mapping for: %s" % (channel_name)
			else:
				channel.setAttribute('deviceType', 'DMX Universe')
				channel.setAttribute('circuit', dmxId)
				if channel.hasAttribute('unit'):
					channel.removeAttribute('unit')
	dom.writexml(open(file_name, 'w'))
	#dom.writexml(open('updated.xml', 'w'))

def Main():
    if len(sys.argv) < 2:
        print 'Usage:'
        print '  python ApplyChannelMapping.py <mappings.csv> <sequence.lms>'
        return -1

    dmxDict = LoadDmxMappings(sys.argv[1])
    ProcessFile(dmxDict, sys.argv[2])


if __name__ == '__main__':
    sys.exit(Main())

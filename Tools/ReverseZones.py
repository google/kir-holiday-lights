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

from xml.dom.minidom import parse


def SwapChildren(channel_map, from_name, to_name):
    from_channel = channel_map[from_name]
    to_channel = channel_map[to_name]

    from_children = from_channel.childNodes[:]
    to_children = to_channel.childNodes[:]
    for child in from_children:
        from_channel.removeChild(child)
    for child in to_children:
        to_channel.removeChild(child)

    for child in from_children:
        to_channel.appendChild(child)
    for child in to_children:
        from_channel.appendChild(child)


def getText(nodelist):
    rc = []
    for node in nodelist:
        if node.nodeType == node.TEXT_NODE:
            rc.append(node.data)
    return ''.join(rc)


def ProcessFile(file_name):
    dom = parse(file_name)
    sequence = dom.getElementsByTagName('sequence')[0]
    channels = sequence.getElementsByTagName('channels')[0]
    channel_map = {}
    for channel in channels.getElementsByTagName('channel'):
        channel_name = channel.getAttribute('name')
        if channel_name:
            print 'Found channel: %s' % channel_name
            channel_map[channel_name] = channel

    for zone in range(1, 7):
        for ground in range(1, 5):
            for rgb in 'RGB':
                from_name = 'Zone %d - Ground %d (%s)' % (zone, ground, rgb)
                to_name = 'Zone %d - Ground %d (%s)' % (zone, 9 - ground, rgb)
                SwapChildren(channel_map, from_name, to_name)

    dom.writexml(open('updated.xml', 'w'))

def Main():
    if len(sys.argv) < 2:
        print 'Usage:'
        print '  python ReverseZones.py <sequence.lms>'
        return -1

    ProcessFile(sys.argv[1])


if __name__ == '__main__':
    sys.exit(Main())

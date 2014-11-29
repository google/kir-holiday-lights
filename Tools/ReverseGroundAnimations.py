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

from xml.dom.minidom import parse
from collections import namedtuple

GroundRing = namedtuple("GroundRing", ["zone", "ring"])


# <rgbChannel totalCentiseconds="20653" name="Zone 2 - Ground 2" savedIndex="82">
def ProcessFile(file_name):
    dom = parse(file_name)
    sequence = dom.getElementsByTagName('sequence')[0]
    channels = sequence.getElementsByTagName('channels')[0]

    ring_to_index = {}
    index_to_ring = {}

    for channel in channels.getElementsByTagName('rgbChannel'):
        channel_name = channel.getAttribute('name')
        # name="Zone (\d) - Ground (\d)" savedIndex="(\d+)"
        if channel_name:
            name_match = re.search('Zone (\d) - Ground (\d)', channel_name)
            if name_match:
                gr = GroundRing(zone=int(name_match.group(1)), ring=int(name_match.group(2)))
                channel_index = int(channel.getAttribute('savedIndex'))
                print 'Found ground channel: %s i: %s' % (gr, channel_index)
                ring_to_index[gr] = channel_index
                index_to_ring[channel_index] = gr

    # Reverse channel order in ground ring animations
    animation = dom.getElementsByTagName('animation')[0]
    for column in animation.getElementsByTagName('column'):
        from_index = int(column.getAttribute('channel'))
        gr_from = index_to_ring.get(from_index, None)
        if gr_from:
            gr_to = GroundRing(zone=gr_from.zone, ring=abs(gr_from.ring - 9))
            to_index = ring_to_index[gr_to]
            print 'Swap: %s:%s -> %s:%s' % (str(gr_from), from_index, str(gr_to), to_index)
            column.setAttribute('channel', str(to_index))

    dom.writexml(open('updated.xml', 'w'))

def Main():
    if len(sys.argv) < 2:
        print 'Usage:'
        print '  python ReverseZones.py <sequence.lms>'
        return -1

    ProcessFile(sys.argv[1])


if __name__ == '__main__':
    sys.exit(Main())

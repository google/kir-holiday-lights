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

#
# Group similar sections together...  Used this to group all the
# Zone * Group [1-8] rings together.
#
import sys

import xml.dom.minidom as md


def getText(nodelist):
    rc = []
    for node in nodelist:
        if node.nodeType == node.TEXT_NODE:
            rc.append(node.data)
    return ''.join(rc)


def AddChannelGroup(dom, parent, saved_index, tag):
    node = dom.createElement(tag)
    node.setAttribute('savedIndex', str(saved_index))
    parent.appendChild(node)


def BuildChannelMap(channels):
    channel_map = {}
    for tag_name in ('channel', 'rgbChannel'):
        for channel in channels.getElementsByTagName(tag_name):
            channel_name = channel.getAttribute('name')
            if channel_name:
                channel_map[channel_name] = channel

    return channel_map


def FindUsedIndices(node, used_indices=None):
    if used_indices is None:
        used_indices = set()

    try:
        used_indices.add(int(node.getAttribute('savedIndex')))
    except (AttributeError, ValueError):
        pass
    for child in node.childNodes:
        FindUsedIndices(child, used_indices)

    return used_indices


def NextUnusedIndex(used_indices):
    used_list = sorted(list(used_indices))
    check = used_list[0]
    while check in used_indices:
        check += 1
    used_indices.add(check)
    return check


def ProcessFile(file_name):
    dom = md.parse(file_name)
    sequence = dom.getElementsByTagName('sequence')[0]
    channels = sequence.getElementsByTagName('channels')[0]
    channel_map = BuildChannelMap(channels)
    used_indices = FindUsedIndices(channels)

    tracks = sequence.getElementsByTagName('tracks')[0]
    total_centiseconds = tracks.getElementsByTagName(
        'track')[0].getAttribute('totalCentiseconds')

    # Create a new track for these groups.
    new_track = dom.createElement('track')
    tracks.appendChild(new_track)
    new_track.setAttribute('totalCentiseconds', str(total_centiseconds))
    new_track.setAttribute('timingGrid', '0')
    new_track.setAttribute('name', 'Coordinated Rings')

    loop_levels = dom.createElement('loopLevels')
    new_track.appendChild(loop_levels)

    track_channels = dom.createElement('channels')
    new_track.appendChild(track_channels)

    # Create the new group list to include all the new channel groups.
    for ground in range(1, 9):
        group_list = dom.createElement('channelGroupList')
        channels.appendChild(group_list)
        group_list.setAttribute('totalCentiseconds', str(total_centiseconds))
        group_list.setAttribute('name', 'All Ground %d' % ground)
        group_list.setAttribute('savedIndex', str(NextUnusedIndex(used_indices)))
        print 'Creating group %s with index %s' % (
            group_list.getAttribute('name'),
            group_list.getAttribute('savedIndex'))

        # Add the new group to the new track.
        AddChannelGroup(dom, track_channels, group_list.getAttribute('savedIndex'), 'channel')

        channel_group = dom.createElement('channelGroups')
        group_list.appendChild(channel_group)

        # Add channels to the group.
        for zone in range(1, 7):
            name = 'Zone %d - Ground %d' % (zone, ground)
            channel = channel_map[name]
            channel_index = channel.getAttribute('savedIndex')
            AddChannelGroup(dom, channel_group, channel_index, 'channelGroup')

    dom.writexml(open('updated.xml', 'w'))

def Main():
    if len(sys.argv) < 2:
        print 'Usage:'
        print '  python GroupSections.py <sequence.lms>'
        return -1

    ProcessFile(sys.argv[1])


if __name__ == '__main__':
    sys.exit(Main())

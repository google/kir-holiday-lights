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
# Convert an Audacity label .txt file to a new timing grid in a
# Light O'Matic sequence file.
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


def LoadLabels(label_file):
    labels = []
    for line in open(label_file):
        if line.strip():
            start_time = line.split('\t', 1)[0]
            labels.append(float(start_time))
    return labels


def GetNextSaveId(timing_grids):
    highest = -1
    for timing_grid in timing_grids.getElementsByTagName('timingGrid'):
        save_id = int(timing_grid.getAttribute('saveID'))
        highest = max(highest, save_id)
    return highest + 1


def ProcessFiles(label_file, sequence_file):
    labels = LoadLabels(label_file)
    dom = md.parse(sequence_file)
    sequence = dom.getElementsByTagName('sequence')[0]
    timing_grids = sequence.getElementsByTagName('timingGrids')[0]
    save_id = GetNextSaveId(timing_grids)

    timing_grid = dom.createElement('timingGrid')
    timing_grids.appendChild(timing_grid)
    timing_grid.setAttribute('saveID', str(save_id))
    timing_grid.setAttribute('name', 'Audacity Timings')
    timing_grid.setAttribute('type', 'freeform')
    for label_time in labels:
        timing = dom.createElement('timing')
        timing_grid.appendChild(timing)
        timing.setAttribute('centisecond', str(int(round(label_time * 100))))

    dom.writexml(open('updated.xml', 'w'))

def Main():
    if len(sys.argv) < 3:
        print 'Usage:'
        print '  python AudacityLabelsToTimingGrid.py <labels.txt> <sequence.lms>'
        return -1

    ProcessFiles(sys.argv[1], sys.argv[2])


if __name__ == '__main__':
    sys.exit(Main())

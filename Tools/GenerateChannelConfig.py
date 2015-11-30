#!/usr/bin/env python

import sys

import xml.dom.minidom as md
import yaml

savedIndex = 0

dom = md.parse("sequence_template.xml")
sequence = dom.getElementsByTagName('channelConfig')[0]
channels = sequence.getElementsByTagName('channels')[0]

tracks = sequence.getElementsByTagName('tracks')[0]
track = tracks.getElementsByTagName('track')[0]
trackChannels = track.getElementsByTagName('channels')[0]

# <rgbChannel totalCentiseconds="6000" name="Zone C - Tree C7 - Base Color" savedIndex="27">
def AddRGBChannel(name, color=12615744, circuit=None):
    global savedIndex
    rgbChannel = dom.createElement('rgbChannel')
    rgbChannels = dom.createElement('channels')
    rgbChannel.appendChild(rgbChannels);

    # Add Red Channel
    subChannel = dom.createElement('channel')
    idx = AddSimpleChannel(name + " (R)", 255)
    subChannel.setAttribute("savedIndex", str(idx))
    rgbChannels.appendChild(subChannel)

    # Add Green Channel
    subChannel = dom.createElement('channel')
    idx = AddSimpleChannel(name + " (G)", 65280)
    subChannel.setAttribute("savedIndex", str(idx))
    rgbChannels.appendChild(subChannel)
    
    # Add Blue Channel
    subChannel = dom.createElement('channel')
    idx = AddSimpleChannel(name + " (B)", 16711680)
    subChannel.setAttribute("savedIndex", str(idx))
    rgbChannels.appendChild(subChannel)
    
    # Add Parent RGB Channel
    rgbChannel.setAttribute("totalCentiseconds", str(6000))
    rgbChannel.setAttribute("name", name)
    rgbChannel.setAttribute("savedIndex", str(savedIndex))
    channels.appendChild(rgbChannel)

    setIdx = savedIndex;
    savedIndex += 1
    return setIdx

# <channel name="Snow Machine 1" color="12615744" centiseconds="6000" deviceType="DMX Universe" circuit="9" network="1" savedIndex="312"/>
def AddSimpleChannel(name, color=12615744, circuit=None):
    global savedIndex
    channel = dom.createElement('channel')
    channel.setAttribute("name", name)
    channel.setAttribute("color", str(color))
    channel.setAttribute("centiseconds", str(6000))
    channel.setAttribute("deviceType", "DMX Universe")
    if (circuit != None):
        channel.setAttribute("network", 1)
        channel.setAttribute("circuit", circuit)
    channel.setAttribute("savedIndex", str(savedIndex))
    channels.appendChild(channel)

    setIdx = savedIndex;
    savedIndex += 1
    return setIdx


# Add a Channel
def AddChannel(name, rgb=False, circuit=None):
    if (rgb):
        return AddRGBChannel(name=name, circuit=circuit)
    else:
        return AddSimpleChannel(name=name, circuit=circuit)

# Add a channel group list
def AddChannelGroup(name, indicies):
    global savedIndex

    cgl = dom.createElement('channelGroupList')
    cgl.setAttribute("totalCentiseconds", str(6000))
    cgl.setAttribute("name", name)
    cgl.setAttribute("savedIndex", str(savedIndex))
    
    cgs = dom.createElement('channelGroups')
    
    for idx in indicies:
        cg = dom.createElement("channelGroup")
        cg.setAttribute("savedIndex", str(idx))
        cgs.appendChild(cg)

    cgl.appendChild(cgs)
    channels.appendChild(cgl)

    setIdx = savedIndex;
    savedIndex += 1
    return setIdx



#Main Logic
with open("/Users/edalquist/Google Drive/KIR Lights/LOR/Sequences/2015/channel_template.yaml", 'r') as stream:
    cfg = yaml.load(stream)

    trackEverything = [];

    # pre-reserved channels
    cgIndexList = [];
    for n in xrange (1, cfg['pre_reserved'] + 1):
        idx = AddChannel("Reserved %d" % n)
        cgIndexList.append(idx);

    # Add Reserved channel group in Everything track config
    trackEverything.append(AddChannelGroup(name="Reserved", indicies=cgIndexList))

    # Effects
    cgIndexList = []
    for effect in cfg['effects']:
        for n in xrange (1, effect['count'] + 1):
            idx = AddChannel(name="%s %d" % (effect['name'], n), rgb=effect['rgb'])
            cgIndexList.append(idx);

    # Add Effects channel group in Everything track config
    trackEverything.append(AddChannelGroup(name="Effects", indicies=cgIndexList))

    # Zones
    for zone in cfg['zones']:
        name = zone['name']
        # Tree Templates
        cgIndexList = []
        for template in cfg['tree_channels_template']:
            # Trees in zone
            for n in xrange (1, zone['tree_count'] + 1):
                idx = AddChannel(name="Zone %s - Tree %s%d - %s" % (name, name, n, template['name']), rgb=template['rgb'])
                cgIndexList.append(idx);
        if zone['tree_count'] > 0:
            # Add Trees channel group in Everything track config
            trackEverything.append(AddChannelGroup(name="Zone %s Trees" % name, indicies=cgIndexList))

        # Ground channels in zone
        cgIndexList = []
        for n in xrange (1, zone['ground_channels'] + 1):
            idx = AddChannel(name="Zone %s - Ground %d" % (name, n), rgb=True)
            cgIndexList.append(idx);
        # Ground templates in zone, these apply globally instead of to each ground channel like for trees
        if (zone['ground_channels'] > 0):
            for template in cfg['ground_channels_template']:
                idx = AddChannel(name="Zone %s - Ground %s" % (name, template['name']), rgb=template['rgb'])
                cgIndexList.append(idx);

            # Add Ground channel group in Everything track config
            trackEverything.append(AddChannelGroup(name="Zone %s Ground" % name, indicies=cgIndexList))


    for topIndex in trackEverything:
        tChannel = dom.createElement('channel')
        tChannel.setAttribute("savedIndex", str(topIndex))
        trackChannels.appendChild(tChannel)


print(dom.toprettyxml())






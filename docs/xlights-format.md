# xLights files

How the files in `xlights/` are structured, and which hand edits xLights
accepts unchanged. Each rule here was checked by a round trip: predict the
edit, make it, load and save in xLights, and compare. A rule marked
*observed* has been seen in the files but not yet confirmed that way.

An edit these rules cover can be made by hand. An edit they don't cover yet
gets a round trip, and whatever differed from the prediction goes in here, so
more edits can be made by hand over time.

Layout and controller rules confirmed with xLights 2026.19, except multiple
strings, smart receivers, Custom model grids, positions, viewpoints and
`AutoSize`, which were confirmed with 2026.20; sequence and automation
rules with 2026.20.

## Editing by hand

- xLights writes one element per line, with each element's attributes in a
  fixed order of its own. A text edit that changes only attribute values, or
  removes an attribute, survives a load and save byte for byte. An added
  attribute survives only if it is put where xLights would write it.
- A model's attributes are written as `name`, `DisplayAs`, then the rest in
  case-insensitive alphabetical order, with `CustomModelCompressed` last.
- Positions (`WorldPosX` and the like) are kept as 32-bit floats and written
  with six decimals, so a computed value comes back with its last digits
  changed (`997.177210` is saved as `997.177185`). Round to single precision
  before writing, or take the values xLights saves.
- Close xLights before editing a file it has open. It saves its in-memory
  copy over the file.
- To round-trip a hand edit, open the show in xLights, make the file dirty
  (change a setting and change it back), save, and diff against the edited
  file. Any difference means the edit wasn't what xLights would write.
- Saving with a newer xLights rewrites parts of the files into its own format
  (new default attributes, renamed values). Expect that in the diff beside the
  edit.

## Layout: `xlights_rgbeffects.xml`

```
<xrgb>
  <models type="rgb_effects">
    <model name="F2-1" DisplayAs="Poly Line" Controller="Fence 45"
           StartChannel="!Fence 45:1" NodesPerString="74" ...>
      <ControllerConnection Port="1" Protocol="ws2811" nullNodes="26" colorOrder="BGR"/>
    </model>
    ...
  </models>
  <modelGroups> ... </modelGroups>
  <settings> ... </settings>
</xrgb>
```

### Models

- `Controller` names a controller in `xlights_networks.xml`.
- `StartChannel="!<controller>:<n>"` is the model's first channel, counted
  from 1 within that controller. *Observed.*
- Node count: `NodesPerString` (Single Line, Poly Line); other model types use
  their own attributes (`NodesPerArch` for Arches).
- Changing a node count by hand means moving the `StartChannel` of every later
  model on that controller, and the controller's `MaxChannels` in
  `xlights_networks.xml`. With `AutoSize="1"` that moves every later
  controller's channels too, so their outputs need uploading again and every
  sequence needs re-rendering. *Observed:* xLights saves the hand edit
  unchanged, and its port map agrees. Let them move: keeping spare channels
  on a controller breaks playback (see "Controllers").
- A Custom model's grid is `CustomModel`: rows top to bottom separated by
  `;`, cells in a row by `,`, layers by `|`, with `CustomWidth`,
  `CustomHeight` and `Depth` giving the size. The model is drawn
  `CustomWidth × CustomHeight × Depth` units before scaling, so changing
  `CustomHeight` changes its drawn height unless `ScaleY` changes in
  proportion. The Pixicles went from height 16 to 15 with `ScaleY` 0.166700
  to 0.177813, and xLights saved the edit unchanged.

### Controller connection

Every model has one `<ControllerConnection>` child element. Some models also
have other children, such as `<subModel>`.

| Attribute | Meaning |
|---|---|
| `Port` | Controller output port, from 1. |
| `Protocol` | Pixel protocol, e.g. `ws2811`. |
| `nullNodes` | Unlit nodes before the model's first node on the port. |
| `endNullNodes` | Unlit nodes after the model's last node. |
| `colorOrder` | e.g. `BGR`. |
| `Reverse` | `1` runs the model's nodes in reverse. xLights also reads the old name `reverse` and writes it back as `Reverse`. |
| `SmartRemote` | Smart receiver on the port: `1` is A, `2` is B, and so on. See "Smart receivers". |
| `SRMaxCascade` | How many receivers the model's strings may spread over. |
| `SRCascadeOnPort` | `TRUE` or `FALSE`. See "Smart receivers". |
| `SmartRemoteType` | Receiver protocol, e.g. `fpp_v2`, `falcon_v1`. |

- The smart receiver attributes come right after `Protocol`, in the order
  above: `SmartRemote`, `SRMaxCascade`, `SRCascadeOnPort`, `SmartRemoteType`.
  xLights writes all four when `SmartRemote` is set and none otherwise.

- A null count of 0 is written by leaving the attribute out, never as
  `nullNodes="0"` or `endNullNodes="0"`.
- Null nodes are sent by the controller and use no channels, so changing them
  doesn't move any `StartChannel` and doesn't change a rendered sequence.
  *Observed:* F2-1 on Fence 45 starts at channel 1 with 74 nodes, and F2-2
  starts at 223 (1 + 74 × 3).

### Several models on one port

- `ModelChain=">X"` means the model follows model `X` on the same port. The
  first model on a port has no `ModelChain`. *Observed.*
- Each model in a chain keeps its own `nullNodes` and `endNullNodes`, so nulls
  can sit between models on a port. *Observed* on Fence 43, port 13.

### Deleting a model or group

- Remove its `<model>`, and its name from the `models` of every
  `<modelGroup>`. On an `AutoSize="1"` controller, lower `MaxChannels` by the
  model's channels. If it wasn't the controller's last model, the later
  models' `StartChannel`s move down as they do for a change of node count.
- A group is its `<modelGroup>` element and closing tag, with no channels.
- In every sequence that lists it, remove its `<Element>` from
  `<DisplayElements>` and its `<Element>` from `<ElementEffects>`. To move a
  group's effects to another group not yet in the sequence, rename both
  `<Element>`s instead.
- *Observed* with the last model on an `AutoSize="1"` controller, and with a
  group whose effects moved to an identical group: xLights loaded the hand
  edit and saved the layout and controllers unchanged, and the sequences
  with the same elements.

### Several strings in one Custom model

A Custom model is split into strings by giving the node each string starts
at:

    CustomStrings="4" ... NodeStart1="1" NodeStart2="129" NodeStart3="193" NodeStart4="257"

- `CustomStrings` sits alphabetically between `CustomHeight` and
  `CustomWidth`, and `NodeStart1`…`NodeStartN` between `Locked` and
  `PixelSize`. Leave all of them out for a single string.
- String 1 goes to `Port`, and each later string to the next port.
- The start nodes don't have to ascend. Each string runs from its start node
  up to the next higher start node, or to the last node. So the order of the
  strings on the ports can differ from the order of the nodes:
  `NodeStart1="1" NodeStart2="193" NodeStart3="65" NodeStart4="129"` on a
  320-node model puts nodes 1–64 on the first port, 193–320 on the second,
  65–128 on the third and 129–192 on the fourth (checked with
  `getControllerPortMap`). The model's channels don't change.
- Splitting a model into strings changes only which port each node is sent on.
  The model's channels and buffer stay the same, so sequences render to the
  same `.fseq` data. Checked by rendering Candy Cane Lane with Gift Box 1 as
  one string and as four: the decoded frame data was identical.

### Smart receivers

A smart receiver takes one differential port and drives four outputs of its
own. Several receivers can be chained on one port as A, B, C and so on.

- xLights counts a receiver's four outputs as four consecutive ports, in
  blocks of four: ports 1–4, 5–8 and so on. A model on a receiver has `Port`
  set to the first port of a block, plus `SmartRemote`.
- A multi-string model with `SmartRemote="1"`, `SRMaxCascade="1"` and
  `SRCascadeOnPort="FALSE"` on `Port="1"` puts strings 1–4 on receiver A's
  outputs 1–4. Checked on a K64D-B with `getControllerPortMap`: Gift Box 1's
  strings were on ports 1–4, receiver A, with 128, 64, 64 and 64 pixels.
- A string that would go past the end of a block moves to the first output
  of the next receiver (`SRMaxCascade` > 1) or to the next block. So start a
  four-string model at the first port of a block.
- xLights refuses a block of four ports that mixes models on receivers with
  models without one. All receivers on a port must be the same type.
- The controller's board definition lists the receiver types it supports.
  Boards that use FPP's shift-register string driver (K16-Max, K64D-B and
  the K16A-B 2.x/3.x) take `falcon_v1`, `fpp_v2`, `falcon_v4` and
  `falcon_v5`. FPP "Pi Hat / 2 Output RPIWS281X" takes none.

### Submodels

A submodel names a set of its model's nodes, so effects can target them
alone. It is a `<subModel>` child after `<ControllerConnection>`:

    <subModel name="Spiral 1" bufferstyle="Default" layout="horizontal" line0="1,400,402-405" line1="..." type="ranges"/>

- Each `lineN` lists 1-based model node numbers, single or as `a-b` ranges,
  joined by commas. With `layout="horizontal"` and the `Default` buffer
  style, each line is one row of the submodel's buffer, its nodes left to
  right in the order listed.
- Submodels use no channels, so adding one changes no `StartChannel` and no
  rendered sequence.
- Nodes can be listed in any order, so a submodel can follow a path across
  the model's own strings. GigaTree's `Spiral 1`–`Spiral 4` are four
  strands winding five times round the tree, one node per string per turn;
  `Spirals` has all four as four lines. xLights saved them unchanged.

Model node numbers on GigaTree (a Tree is a vertical matrix): node
`s × 200 + y + 1` (string `s` and `y` from 0) is in buffer column `s` (or
`31 − s`, depending on `Dir`), and with `AlternateNodes="true"` and
`StartSide="B"` in buffer row `2y` for `y` < 100 and `2(199 − y) + 1` above
that. Row 0 is the bottom. `exportFirstStrand` moves only the channels, not
the node numbers.

### Spiral trees

`TreeSpiralRotations` on a Tree is how many times each string goes round
the tree, bottom to top, and its sign is the direction. Seen from above, a
positive value winds counter-clockwise going up and a negative one
clockwise; the strings start at the same places at the base either way.
*From `TreeModel::SetTreeCoord`*, which uses the value as a signed angle step;
xLights reads it with no clamp. It spreads the turns roughly by arc length
(ten segments, rounded to whole pixels), so a string goes round faster near
the narrow top.

On a 360° tree laid on its back (`RotateX="90"`, as here) the strings start
360°/`NumStrings` apart, in buffer-column order: column `c` starts at bearing
`90° − (c × 360°/NumStrings − 180° + TreeRotation)` (clockwise from +X, seen
from above). With `Dir="L"` string `s` (from 0) is column `s`, so successive
strings start counter-clockwise of each other; with `Dir="R"` it is column
`NumStrings − 1 − s`, so they go clockwise. With four strings and `Dir="R"`,
string 1 starts at bearing `−TreeRotation`.

The North Trees are `Dir="R"`: each tree's four strings start a quarter turn
apart going clockwise, the same way they wind, go round about twice in
pixels 1–42 and bunch into a tip in pixels 43–50. The trees are wound by
hand, so `TreeSpiralRotations` (−2 to −3.5) and `TreeRotation` (where
string 1 starts) are fitted per tree to video of comets climbing each
string: the pixel at which each string's comet crosses the middle of the
face toward the camera, with the camera's bearing from each tree worked
out from the trees' positions in the frame. A dusk clip is needed; in
daylight the green and white comets don't show.

### Viewpoints

Saved 3D viewpoints are `<Camera>` elements in `<Viewpoints>` at the end of
the layout, before `<DefaultCamera3D name="DEFAULT3D">`:

    <Camera name="Walkway Tunnel" posX="0" posY="0" posZ="0" angleX="271.487" angleY="0" angleZ="291.318" distance="-1533.86" zoom="0.10046" panx="-479.5" pany="38.7" panz="-12" zoom_corrx="0" zoom_corry="0" is_3d="1"/>

- World axes are the layout's: X and Y on the ground (north is +X and west
  is +Y in this show), Z up. The view is
  `T(1, 1, distance × zoom) · Rx(angleX) · Ry(angleY) · Rz(angleZ) · T(pos + pan)`
  with a 45° vertical field of view; the width follows the window.
- So the camera looks at the point −(pos + pan) from `distance × zoom` away.
  All angles 0 looks straight down; `angleX` 270 looks level. With
  `angleY` 0 the horizon stays level, and `angleZ` turns the view: for an eye
  at E looking at T, with d = E − T, h = |d| along the ground and
  L = |d|, `angleX` = atan2(−h/L, d_z/L), `angleZ` = atan2(−d_x, −d_y),
  `pan` = −T and `zoom` = L / −`distance`.
- The layout's scale is about 10 units to the metre (the GigaTree's 27 ft of
  lights is 82.4 units), so eye height on a walkway is about 16.
- Viewpoints are listed by name in the Layout tab's right-click menu (Load
  ViewPoint). Hand-written ones survived a load and save unchanged, and
  loading them showed the view the formula predicts.

## Controllers: `xlights_networks.xml`

```
<Networks ...>
  <Controller Id="45" Name="Fence 45" Vendor="KulpLights" Model="K16"
              Variant="K16A-B 1.x" IP="192.168.1.45" Protocol="DDP" ...>
    <network NetworkType="DDP" MaxChannels="2052" Enabled="Yes" ... />
  </Controller>
</Networks>
```

- `Name` is what models refer to in `Controller` and `StartChannel`.
- xLights keeps the controllers in file order, and that order sets the channel
  numbering: each controller's channels follow the previous one's. Add a new
  controller after the last one, so no other controller's channels move.
- With `AutoSize="1"`, `MaxChannels` follows the models on the controller. A
  controller left with no models is written with `MaxChannels="1"`.
- With `AutoSize="0"`, `MaxChannels` stays as written even when the models use
  fewer channels. Don't use it to keep later controllers from moving: a render
  stores only the channels models use (sparse ranges in the `.fseq`), and FPP
  gives an output that asks for channels outside those ranges nothing at all
  ("Requested output range outside read ranges" in `fppd.log`). *Observed:*
  a DDP output of 960 channels to 900 channels of models got no data.
- `Vendor`, `Model` and `Variant` choose the board definition that xLights
  uses to build the controller's port configuration when it uploads. The
  definitions are in xLights' `controllers/*.xcontroller` files.
- Changing `Variant` by hand to another name from those files survives a
  load and save.

### Kulp K16 variants

| `Variant` | Cape | FPP string driver |
|---|---|---|
| `K16A-B 1.x` | version 1.x (cape id `K16A-Bv1`) | PRU strings |
| `K16A-B 2.x/3.x` | version 2.x or 3.x (cape id `K16A-B`) | shift-register strings |

The two drivers differ, so the variant must match the board. xLights reads the
plain name `K16A-B` as `K16A-B 1.x`. Ask a controller which board it has:

    curl http://<address>/api/cape     # "version" and "id"

The show's K16s are 1.x boards, except .41 (version 3.0). .59 and .62–.64
are not yet checked.

## Sequences: `*.xsq`

```
<xsequence ...>
  <head> <version>2026.20</version> <sequenceTiming>50 ms</sequenceTiming> ... </head>
  <ColorPalettes>
    <ColorPalette>C_BUTTON_Palette1=#FF0000,...,C_CHECKBOX_Palette1=1,...</ColorPalette>
  </ColorPalettes>
  <EffectDB>
    <Effect></Effect>
    <Effect>B_CHOICE_BufferStyle=Single Line,...,E_SLIDER_Marquee_Band_Size=1,...</Effect>
  </EffectDB>
  <DisplayElements> <Element type="model" name="A1-1" ... /> ... </DisplayElements>
  <ElementEffects>
    <Element type="model" name="A1-1">
      <EffectLayer>
        <Effect ref="2" name="Marquee" startTime="0" endTime="60000" palette="1" />
      </EffectLayer>
    </Element>
  </ElementEffects>
</xsequence>
```

- An effect in `<ElementEffects>` carries no settings. `ref` indexes
  `<EffectDB>` and `palette` indexes `<ColorPalettes>`, both from 0. Effects
  with identical settings share one entry. Times are in milliseconds.
- Saving rebuilds both tables from the effects in use: unused entries are
  dropped and the rest renumbered in order of first use in
  `<ElementEffects>`. A hand edit can append new entries at the end; xLights
  loads it and only renumbers on its next save.
- Settings are `key=value` pairs joined by commas, keys sorted. `E_` keys
  belong to the effect, `B_` keys to its buffer, `C_` keys to the palette.
- A palette lists colours as `C_BUTTON_Palette<n>` and marks the ones in use
  with `C_CHECKBOX_Palette<n>=1`. Effects use the checked colours in order.
- A palette slot can hold a colour curve instead of a colour:
  `C_BUTTON_Palette1=Active=TRUE|Id=ID_BUTTON_Palette1|Timecurve=2|Values=x=0.000^c=#b4c8d7;x=1.000^c=#3264a0|`.
  Points are `x=<0–1, three decimals>^c=<colour>` joined by `;`, and the
  value ends with `|`. xLights writes colours it saves as lowercase
  `#rrggbb`; it also reads names (`green`) and `rgb(230@ 115@ 185)`, which
  some sequences here use.
  Without `Timecurve` the colour changes over the effect's time.
  `Timecurve` makes it a gradient across the buffer instead: 1 left to right,
  2 top to bottom, 3 right to left, 4 bottom to top, 5 and 6 radial in and
  out, 7 and 8 clockwise and anticlockwise. Effects that support spatial
  colour, such as On, use it.
- `<sequenceDuration>` is the length in seconds with three decimals. To
  change the length of a sequence without media, change it and the
  `endTime` of every effect that should run to the end; xLights loads and
  saves that unchanged.
- `<mediaFile>` is a path from whichever computer last chose the audio;
  xLights finds the file by its name in the media folder. To change a
  sequence's audio, change the file name there, set `<sequenceDuration>` to
  the new audio's length (`ffprobe`'s Duration), and move every effect that
  should end with the song, as above; effects end on a whole frame, at or
  after the length (`183.640` s ended at `183650`). *Observed* in headless
  renders of Peanuts with another recording swapped in, which
  also take the `.fseq`'s media name from it.
- On a layer with several effects, the first `<Effect>` has no `id` and the
  rest have `id="1"`, `id="2"` … in order, before `startTime`.
- Effects on a submodel go in a `<SubModelEffectLayer name="<submodel>">`
  inside the model's `<Element>`, after its `<EffectLayer>`s, holding
  `<Effect>`s directly. `<DisplayElements>` doesn't list submodels.
- A submodel's further layers are more `<SubModelEffectLayer>`s with a
  `layer` attribute before `name`: `<SubModelEffectLayer layer="1"
  name="Spiral 1">`. Layer 0 has no `layer` attribute. Each submodel's
  layers are written together, in layer order, before the next submodel's.
- Submodel effects render after the model's own layers. With
  `ModelBlending="true"` on `<xsequence>` they blend over what the model's
  layers drew, so black in a submodel effect shows the model through.
  Submodels render one after another in the order the model defines them,
  and each one's layers mix, by their own layer methods, onto everything
  drawn before it: the model and the earlier submodels. A later submodel
  that shares nodes with earlier ones can draw over them.
- A model's `<EffectLayer>`s are listed top layer first. The effect on a
  layer says how it mixes with the layers below it with
  `T_CHOICE_LayerMethod` (`Normal` if absent) and the threshold
  `T_SLIDER_EffectLayerMix` (percent, default 0). `1 is Mask` blacks out the
  layers below wherever this layer is brighter than the threshold.
- Only a `Normal` layer gives its effect an alpha channel: fades and the
  pixels an effect leaves undrawn are transparent. Under every other method
  effects draw their fades as dimmer colours, and undrawn pixels are black
  with zero alpha.
  - `Additive` adds the colours and ignores alpha, so undrawn pixels add
    nothing. It suits light laid over a base: pick the colour as what it
    adds.
  - `Max` multiplies its result by the upper pixel's alpha, so wherever the
    upper effect draws nothing the result is black. Use it only over effects
    that fill their whole buffer.
  - Twinkle outside `Normal` sets each palette colour's brightness to its
    ramp, so a black palette colour twinkles white.
- A pixel masked inside a submodel comes out black, not as what the model's
  own layers drew there.
- `B_CHOICE_BufferStyle` offers different styles per model type. On a Single
  Line model, `Single Line` is the model's own buffer and is stored as
  `Default`.
- Saving a sequence also writes its `.fseq` from what is rendered in memory.
- Saving a sequence last saved by an older xLights rewrites its form: the
  `<version>`, `<?xml version="1.0"?>` without an encoding, empty `<head>`
  elements as `<x></x>`, other empty elements as `<x />`, booleans in
  `<DisplayElements>` as `true`/`false`, `<nextid>` and `<Jukebox/>` dropped,
  `<SequenceMedia />` added, and absolute shader paths made relative to the
  show folder.

### Strand colours with Marquee

Marquee with band size 1, skip 0 and speed 0 colours one node per checked
palette colour, in palette order, repeating, and holds still.

- On a one-row buffer it follows the wiring from the model's first node. Use
  `B_CHOICE_BufferStyle=Single Line` for lines, arches and custom models.
- On a two-dimensional buffer it draws concentric rectangles, one per unit of
  `Thickness`. Thickness equal to half the narrower side fills the buffer;
  more makes rectangles overlap. Every rectangle starts its colours at its
  corner, so the sides line up into rings, unless `E_SLIDER_Marquee_Stagger`
  shifts each one. `TraditionalStaticLights` uses stagger 2 on the trees.
- A tree's wiring order doesn't match its rows and columns: GigaTree has
  `AlternateNodes="true"` (each string runs up and back down the same column),
  and strings of a length divisible by the colour count give rings in Single
  Line style. Trees use their default buffer.
- On a one-line submodel with the `Default` buffer, the colours run from the
  last listed node backwards: palette colour 1 is on the last node.
  `TraditionalStaticLights` uses this on GigaTree's spirals.
- `E_SLIDER_Marquee_Start` (0–100) shifts which palette colour comes first:
  with band 1 and skip 0 the shift is Start modulo the colour count.
- `E_CHECKBOX_Marquee_Reverse` reverses the motion, not the colour order:
  the colours still step forward through the palette. With speed 0 it only
  changes the first colour, like another Start. For a strand plugged in
  from the other end, use a palette with the colours in reverse order.
  `TraditionalStaticLights` gives each strand its own Start (0–4)
  and palette direction.
- A palette has 8 slots and may repeat a colour, so with band 1 and skip 0
  a Marquee repeats every 8 nodes at most. `TraditionalStaticLights` uses red, green,
  blue, amber, green, red, pink, blue (or the reverse): no two neighbouring
  pairs are alike, and red, green and blue come up twice as often as amber
  and pink.
- `E_SLIDER_Marquee_Skip_Size` N leaves N nodes transparent after each
  coloured one, so a skip layer above a strand recolours every (N+1)th node
  and shows the strand through the rest. Start then counts over
  (1+N) × colours nodes: it moves both where the coloured nodes fall and
  which colour comes first.
- `TraditionalStaticLights` puts such a "replaced bulb" layer
  between the Twinkle and each strand: skip 12, palette amber, pink, red,
  green, blue, and a Start from 0, 9, 18 … 63 picked per strand. Over an
  8-colour strand the colours repeat every 520 nodes.

### Bulbs blinking out with Twinkle

A Twinkle effect on the layer above the strand, with
`T_CHOICE_LayerMethod=1 is Mask`, turns individual bulbs off:

    B_CHOICE_BufferStyle=<the strand's>,E_CHECKBOX_Twinkle_ReRandom=0,E_CHECKBOX_Twinkle_Strobe=0,E_CHOICE_Twinkle_Style=New Render Method,E_SLIDER_Twinkle_Count=3,E_SLIDER_Twinkle_Steps=30,T_CHOICE_LayerMethod=1 is Mask

- `Count` is the percentage of the buffer's pixels twinkling at once (2–100),
  or of the nodes when fewer than a fifth of the buffer's pixels are nodes,
  rounded to whole pixels. `Steps` is one twinkle's length in
  frames (2–400), a ramp up and down. Each twinkle starts at a random point
  in its ramp, so with the mask a bulb is off for between 1 and Steps − 1
  frames.
- Twinkle's random numbers are seeded from the model name, the layer and the
  effect's start, so models with the same settings blink differently.
- With `E_CHECKBOX_Twinkle_Strobe=1` a twinkling pixel shows for one frame
  of its cycle. A burst shorter than `Steps` frames lights only the pixels
  whose flash falls inside it. *Observed* in Holly Jolly's 250 ms Pixicle
  bursts (5 frames), Count 30, on a `Single Line Model As A Pixel` buffer
  (one cell per Pixicle): Steps 44 lit almost nothing, Steps 5 flashes two
  or three whole Pixicles per burst.
- The palette doesn't matter; any lit colour masks.
- `TraditionalStaticLights` puts this over every strand: on
  GigaTree's spirals as each spiral's layer 0, with the replaced-bulb layer
  on layer 1 and the strand on layer 2.
  xLights loaded and saved that unchanged apart from renumbering.

### Drips with Morph

Morph moves a line from a start position to an end position over the
effect's time, followed by a tail. Positions are percent of the buffer
(`E_SLIDER_Morph_Start_X1` … `End_Y2`, with Y 100 at the top). A drip down
a whole tube is start Y1 = Y2 = 100, end Y1 = Y2 = 0, X1 = 0 and X2 = 100.

- `E_SLIDER_MorphStartLength` and `MorphEndLength` are the head's length in
  rows at the start and the end.
- `E_SLIDER_MorphDuration` is the percentage of the effect during which the
  head travels. The tail is `total length × (100/Duration − 1)` rows long and
  drains out in the rest of the time.
- `E_SLIDER_MorphAccel` (−10 to 10) shapes the motion: positive starts slow
  and speeds up, like a falling drop.
- With two palette colours, the first is the head and the second the tail.
  The tail fades out over its last half.
- A Custom model with `Depth` above 1 has a default buffer `CustomWidth ×
  Depth` wide and `CustomHeight` tall, its layers side by side. Each Pixicle
  (width 1, height 15, depth 2) is a 2 × 15 buffer: the front face (nodes 1 to
  15, top to tip) and the rear (nodes 30 to 16), so a full-width line lights
  both faces at the same height.
  The TreeTopper (61 × 91, depth 61) is 3721 × 91 for about 210 lights, and
  a group holding it with `layout="horizontal"` (`GigaTreeAndTopper`) puts
  that beside the GigaTree's 32 × 200. *Observed*: Lines on that group
  rendered about 9 s slower per 1200 frames than on any other group, and
  Circles there lit 5.5% of the GigaTree and left the star dark 82% of the
  time. On the GigaTree's own buffer and the star's `Per Preview` (camera
  `2D`, so seen from above) both were always lit, the star in one colour.
- `TraditionalStaticLights` gives each Pixicle drips on layer 0
  (head white, tail `#A0DCFF`, duration 75, acceleration 3, 0.8–1 s each, at
  the tube's own rate of one every 5–10 s) over an On on layer 1 with a
  gradient from `#b4c8d7` at the top to `#3264a0` at the tip. xLights loaded
  and saved that unchanged.

### Radiant star: SingleStrand, Morph and Twinkle on submodels

TreeTopper is 14 rays of 15 nodes (submodels `Up`, `UpSE` … `Down`, each
wired from the centre, x = 0 in its buffer, out to the tip) and four plane
submodels that each hold 4 or 6 of the rays.

- A SingleStrand chase (`E_NOTEBOOK_SSEFFECT_TYPE=Chase`) with
  `E_CHOICE_Chase_Type1=Left-Right` and `E_SLIDER_Number_Chases=1` runs a
  band from x = 0 to the end, entering and leaving fully.
  `E_SLIDER_Color_Mix1` is the band's length in percent of the buffer.
  `E_TEXTCTRL_Chase_Rotations` (0.1 to 50.0) is the number of passes over
  the effect's length, so faster chases need shorter effects.
  `E_CHOICE_Fade_Type=From Head` keeps the leading pixel brightest.
- A Twinkle lights `round(Count% × pixels)` buffer pixels at a time, each
  ramping up and down over `Steps` frames and replaced by a new random pixel
  when done; a buffer of 15 pixels never has fewer than one twinkling at
  Count 4 and none below that.
- `TraditionalStaticLights` gives each ray, top first: a Morph flare
  (`Additive`, white head, `#FFE0A0` fading to black tail, 0.8 s, duration
  50, acceleration −3) about every 22–40 s at the same times on every ray;
  a SingleStrand pulse (`Additive`, `#806040`, size 60, From Head, 12 passes
  in each 30 s effect, so one every 2.5 s); and an On with a gradient from
  `#d9b27a` at the centre to `#9a5a0a` at the tip (`Timecurve=1`). The
  model's own layer is empty. The four planes, rendered after the rays,
  carry a white Twinkle (`Normal`, Count 2, Steps 8): one or two glints per
  plane at a time, more often on `Up` and `Down`, which are in every plane.
  xLights loaded and saved that unchanged.
- Peanuts fades the show in and out with `Off` effects on the
  top layer of `All Models` and GigaTree. A model's layers don't reach its
  submodels' effects, so the star repeats GigaTree's top layer above each
  ray. A submodel that shares nodes with
  an earlier one and carries the same fade dims those nodes twice: the
  planes would, so their glints run only between the fades instead.

### Gift boxes

Every gift box is the same Custom model, 10 wide, 9 high and 10 deep, laid
upright (`RotateX="90"`): node `(node, row, column, layer)` in
`CustomModelCompressed`. Row 0 is the lid (columns 1–8 by layers 1–8),
layers 0 and 9 the front and back (rows 1–8 by columns 1–8), columns 0 and 9
the sides (rows 1–8 by layers 1–8); the bottom is open. 320 nodes, 64 a face.

Each box has a `Ribbon` submodel: four lines, two nodes wide each way,
up the front, across the lid and down the back (columns 4 and 5), and up
the left side, across the lid and down the right (layers 4 and 5), with
the four nodes where they cross listed once. Peanuts puts an On in the paper
colour on the box and one in the ribbon colour on `Ribbon`.

### Gradients with Bars

- With `E_CHECKBOX_Bars_Gradient=1` each bar blends its colour into the
  next one in whole buffer rows (columns for `Left` and `Right`). A bar is
  `ceil(rows / (E_SLIDER_Bars_BarCount × colours))` rows, the bars move one
  row at a time, and one cycle moves them by a whole palette (`colours`
  bars). The fewer rows the lights cover, the coarser the gradient, both
  across the yard and over time.
- A group's `Per Preview` buffer with camera `2D` is the group's top-down
  extent scaled so that its longer side is the group's `GridSize` (400
  here) cells. Its
  `Default` buffer with `layout="grid"` scales the larger of that extent
  and the preview size (`previewWidth` × `previewHeight` in `<settings>`)
  instead, so the lights can cover only part of it.
- With `layout="minimalGrid"` the `Default` buffer fits the group's own
  lights. A small group on `grid` lights only a corner of its buffer, so
  spatial effects wash it in one colour or miss it. *Observed* on North
  Trees: going from `grid` to `minimalGrid` about
  doubled the colours per frame of Bars, Galaxy and Butterfly in Carol of
  the Bells, and roughly tripled the brightness spread between trees under
  Bars, Morph, Shockwave and Galaxy. On rendered the same.
- A group's `Default` buffer (`grid` or `minimalGrid`) always places its
  nodes by their world X and Y, whatever the group's `DefaultCamera`
  (`ModelGroup.cpp`, which builds it as `Per Preview No Offset` with camera
  `2D`). X and Y are the ground here, so the buffer is the yard seen from
  above, and a standing prop (`RotateX="90"`) is edge-on: its height falls
  into a few cells. *Observed* on `All Pixicles` in Carol of the Bells:
  under Bars, Morph and Circles each lit Pixicle shows 1.0–1.2 colours per
  frame, top to tip alike, and setting the group's `DefaultCamera` to
  `Overlook` rendered the same.
- Only `Per Preview` and `Per Model Per Preview` buffers use a camera: the
  effect's `B_CHOICE_PerPreviewCamera` (default `2D`), which projects the
  nodes through that saved viewpoint (`Model.cpp`). A group's
  `DefaultCamera` only preselects that choice when an effect is added in
  the GUI; it isn't read when rendering.
- So the sequences here give every spatial effect on `All Pixicles`
  `B_CHOICE_BufferStyle=Per Preview,B_CHOICE_PerPreviewCamera=Walkway
  Pixicles`, which shows the Pixicles side-on, hanging straight. *Observed*
  against renders on the 2D map: Plasma, Butterfly, Morph and Shockwave
  vary top to tip as much as or more than they did there. Horizontal
  sweeps (Bars `H-expand`, Curtain from an edge) light each Pixicle in one
  colour, where the 2D map's 17° tilt cut across them. Holly Jolly's
  Twinkle bursts are the exception: they flash whole Pixicles on a
  `Single Line Model As A Pixel` buffer.
- Sizes in effect settings are in buffer cells, such as Circles' `Size` (a
  radius) and Lines' `Thickness`, so the same settings draw smaller on a
  bigger buffer. *Observed*: ten circles of `Size` 18 on `Everything`
  (`GridSize` 2000, about 2000 × 830 cells) lit 0.6% of the fence lights on
  average, the share of the buffer their area covers.
- This layout is about 1865 units along X and 773 along Y. Northbound
  Gradient (`All Models`, `Default`, `Right`, 8 colours, BarCount 4) gets
  bars of 13 columns. Peanuts (`Per Preview`, `up`) gets 166
  rows: bars of 6 rows at BarCount 4, and of 21 rows at BarCount 1, which
  also makes its bands about as wide in the yard as Northbound's.
  *Observed* in renders: a light shows about 61 distinct colours per cycle
  at BarCount 4, 181 at BarCount 1, and 124 in Northbound.
- A palette's `C_SLIDER_SparkleFrequency` flashes random lights white. Which
  lights flash depends on the element's name: the same effect on an
  identical group with another name renders the same colours, with the
  sparkles on other lights. *Observed* in Peanuts.

### Zigzags with Wave

A pattern meant to look the same from every side of a 360° tree goes on
the tree's own buffer (no `B_CHOICE_BufferStyle`, or `Default`), whose
columns are the strings, all the way round. A `Per Preview` buffer projects
the pattern through one camera, so it only reads from that side.

`E_CHOICE_Wave_Type=Triangle` draws one zigzag across the buffer, `H` rows
by `W` columns (*from `WaveEffect.cpp`*, matched to renders):

- It climbs `E_SLIDER_Number_Waves / 900` rows per column, whatever its
  height. Its peak-to-peak height is `amp = H × E_SLIDER_Wave_Height / 100`
  rows (rounded down), centred on row `H/2`, so one period is
  `2 × amp / slope` columns. It joins up round a tree when `W × slope` is a
  multiple of `2 × amp`.
- The band reaches `E_SLIDER_Thickness_Percentage × H / 200` rows above and
  below the line. The steps between columns are `slope` rows, so a band
  thicker than that looks continuous.
- `E_TEXTCTRL_Wave_Speed` S moves it by `2 × S` columns a second, right to
  left (towards column 0) with `Right to Left`. Each effect starts the
  movement afresh, from the same place.
- `E_SLIDER_Wave_YOffset` Y (−250 to 250) moves the whole zigzag by
  `H × Y / 200` rows; on GigaTree a positive Y moves it down. *Observed*.
- The slider stops at 3600, 4 rows per column. xLights renders larger values
  from the file as written. Changing the effect in the effect panel may
  clamp the value to 3600.

On GigaTree (32 × 200, about 7 rows to a column's width at mid-height)
4 rows per column is a shallow wobble. Peanuts' zigzag uses 8100
(9 rows per column), height 18 (36 rows), thickness 8 (±8 rows) and speed 1:
four teeth round the tree, turning once every 16 s, in black over a steady
`#ffc800` On on the layer below, which covers the yard's gradient on the
tree. Below the tree's top (fade) layer, in the yellow section the zigzag
is one Wave per beat at speed 0, offset +8 and −8 in turn, so it holds
still and hops.

### The same spot on every model of a group

A group's `Per Model …` buffer styles render the effect on each member
model's own buffer: `Per Model Single Line` gives every model a one-row
buffer in wiring order. `B_CUSTOM_SubBuffer` (`x1xy1xx2xy2`, percent) doesn't
follow: it cuts the group's whole buffer, which for every `Per Model` style
is the group as one single line, so a sub-buffer lights a stretch of the
first models only. *From `PixelBuffer.cpp`*, matched to renders.

To light the same fraction of every model, the effect has to place itself:

- Fill with `E_CHOICE_Fill_Direction=Right`, `E_CHECKBOX_Fill_Offset_In_Pixels=0`
  and `E_CHECKBOX_Fill_Wrap=1` lights from `E_SLIDER_Fill_Offset` percent of
  the buffer for `E_SLIDER_Fill_Position` percent of it (both whole
  numbers).
- Marquee with band 1, skip N and speed 0 on a one-row buffer colours every
  (N+1)th node from the model's first node, shifted by `E_SLIDER_Marquee_Start`,
  and leaves the rest transparent.

The fence panels are wired in pairs out from a shared controller point
(A1 and A2, B1 and B2, …), so on single-line buffers each pair is mirrored
about the point where they meet. Fence nodes are about 14 cm apart and the
four rails 20 cm.

Peanuts' piano notes put these on
the fence rail groups (`East Fence 1`–`4`, `West Fence 1`–`4`), with `On` on
`Tunnel Row 1`–`8` and `Trellis 1`–`16`. A group that isn't in a sequence
yet gets a row at the end of `<DisplayElements>`, in the form of the other
rows, and its `<Element>` at the end of `<ElementEffects>`; it renders after
the models above it, over `All Models`. *Observed* in headless renders, not
yet saved through xLights.

- On a group's `Horizontal Per Model` buffer each active member is one column,
  in the group's member order, so a `B_CUSTOM_SubBuffer` one column wide
  lights one member whole. Peanuts' solo sparks pick single
  tunnel arches this way (`Tunnel` lists them alphabetically).
- `E_SLIDER_On_Transparency` (0–100) makes an On see-through: a black On at
  transparency 25 on a group's bottom layer leaves a quarter of what
  earlier elements drew (the gradient) showing. *Observed* in renders.
- SingleStrand's `E_CHOICE_Chase_Type1=Bounce to Middle` on a `Per Model
  Single Line` buffer runs from both ends of each arch to its top and back,
  once per `E_TEXTCTRL_Chase_Rotations`.
- A group's row renders after its members' own rows above it, so its layers
  cover what they drew. To give one member its own effects under the
  group's background (yellow, dimming), put everything on the members'
  rows: Peanuts' North Forest effects are on `North Tree 1`–`12`,
  and its `North Trees` row is empty.
- On a tree's own buffer (strings wide, nodes per string high, row 0 at the
  bottom), Fill with `E_CHOICE_Fill_Direction=Up`, offset in percent and
  wrap on lights a band of the tree's height on every string: offset 75 and
  position 25 is its top quarter.

## Automation

With the xFade/xSchedule port set to A (`xLights -a`, or Preferences →
Output), xLights accepts commands as JSON POSTed to
`http://localhost:49913/xlDoAutomation`. It listens on every interface, so
turn the port off when it isn't in use.

| Command | Use |
|---|---|
| `{"cmd":"openSequence","seq":"<abs path>.xsq","promptIssues":"false"}` | Open a sequence. Give an absolute path: a bare name can open a file of that name in another folder xLights remembers, even with `-s` pointing at a copy, and saving then writes there. |
| `{"cmd":"getEffectIDs","model":"<model>"}` | Effect ids per layer: `{"effects":[["0"],[]]}`. |
| `{"cmd":"getEffectSettings","model":"<model>","layer":0,"id":0}` | An effect's name, settings, palette and times. |
| `{"cmd":"addEffect","target":"<model>","effect":"Marquee","settings":"<k=v,...>","palette":"<C_...>","layer":0,"startTime":0,"endTime":60000}` | Add an effect. |
| `{"cmd":"deleteEffect","model":"<model>","layer":0,"id":0}` | Delete an effect. The layer stays, empty. |
| `{"cmd":"renderAll"}` | Render the open sequence in memory. |
| `{"cmd":"saveSequence"}` | Save the open sequence and its `.fseq`. |
| `{"cmd":"closeSequence","quiet":"true","force":"true"}` | Close the sequence without saving or asking. |
| `{"cmd":"exportModel","model":"<model>","filename":"<abs path>","format":"<format>"}` | Export one model's rendered output. |
| `{"cmd":"exportVideoPreview","filename":"<abs>.mp4"}` | Video of the House Preview as it is on screen. |
| `{"cmd":"getModels"}`, `{"cmd":"getOpenSequence"}`, `{"cmd":"getShowFolder"}` | Queries. |
| `{"cmd":"getModel","model":"<model>"}` | A model's attributes as JSON, with its `ControllerConnection`. |
| `{"cmd":"getControllerPortMap","name":"<controller>"}` | Each port's models, strings, receivers, pixels and channels. |
| `{"cmd":"saveLayout"}` | Save `xlights_rgbeffects.xml` and `xlights_networks.xml`, dirty or not. |
| `{"cmd":"uploadController","ip":"<address>"}` | Upload a controller's pixel strings, as the GUI's controller upload does. Restarts its fppd. |
| `{"cmd":"uploadSequence","ip":"<address>","seq":"<name>.xsq","media":"false","format":"<format>"}` | Upload the sequence's rendered `.fseq` to an FPP instance, as FPP Connect does. `v2zstdsparse` keeps only the channels in that instance's outputs (for remotes); `v2zstd` is the whole file (for the show player). |
| `{"cmd":"closexLights","force":"true"}` | Quit xLights. |

`exportModel` formats:

| `format` | Output |
|---|---|
| `vixenroutine` | `.vir` text: one line per channel in wiring order, one value per frame. Exact. |
| `losslessrgb` | `.mov` (ProRes 4444) of the model's default buffer, one pixel per node; colours within ±1. The extension is always `.mov`. |
| `mp4compressed` | `.mp4` of the default buffer, scaled up and lossy. |

- `lightsOn`, `upload*` and `playJukebox` drive the live show.
- An upload asks the controller for its config first. If one of those
  requests gets no answer, xLights waits with no timeout and stops answering
  automation. Restart xLights and upload again. Either way, read the
  controller's config back to check: a reply can be lost even when the upload
  went on, and an upload can be skipped even when later ones succeed.
- Don't use `setModelProperty` for controller-connection properties. It
  replays a change through the layout panel's property handler, which can
  crash xLights. Edit the file by hand instead.

## Rendering without the GUI

    xLights -r -s <show dir> -m <media dir> "<show dir>/<sequence>.xsq"

writes `<sequence>.fseq` next to the sequence and exits. The file is sparse:
its header lists the channel ranges that have models, and only those are
stored, so a frame is shorter than the show's channel count. Give the sequence as
an absolute path: with a bare name it fails to load, then stops at an "Unable
to create file" dialog. It also creates `RenderCache/` and rewrites
`xlights_rgbeffects.xml` (same content, new timestamp), so run it on a copy
of the show directory.

`xLights -r` renders at normal definition. The show's sequences are rendered
with Batch Render's Force High Definition, which changes the output (in
Holly Jolly about 0.2% of channel values, most of them by 64 or more). To get
the same file without the GUI dialog, drive xLights through automation:
`openSequence`, then `renderAll` with `"highdef": "true"`, then
`saveSequence`, which writes the `.fseq`. *Observed:* that reproduced every
channel value of the show player's Holly Jolly; `xLights -r` differed in
307854 of them.

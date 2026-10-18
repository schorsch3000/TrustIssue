# The case

A 3D printed case holds the Nano, the display and the button, so the device can sit on a desk or travel in a pocket or backpack.
It consists of three printed parts and two screws. The models are in the repository's
[`Case/`](https://github.com/schorsch3000/TrustIssue/tree/master/Case) directory.

```{image} /_static/device/device.jpg
:alt: The finished device: a black case with a red button on the left and the display in the middle, connected by USB.
:align: center
```

## What you need

| Part                  | Notes                                                                                  |
| --------------------- | -------------------------------------------------------------------------------------- |
| `body.stl`            | the outer shell, with the opening for the button and the window for the display        |
| `mid.stl`             | the carrier that holds the display and the button and has the Nano mounted on its back |
| `lid.stl`             | the lid that closes the case                                                           |
| 2 screws              | M2, 5 to 15 mm long                                                                    |
| the button            | a generic 12 mm tactile push button with a cap                                         |
| the electronics       | the parts and wiring of {doc}`wiring`                                                  |

`lid.stl` has two voids for cylindrical magnets (15 x 3 mm). If you want to use them, pause the print before the
layer that closes the voids, insert the magnets and resume. The magnets are optional: the case works without
them, but with them the device stays where you put it on a metal surface, or on another 3D printed part that holds two
more magnets. The voids are slightly larger than the magnets, so the lid can be printed over them without errors.

## Printing

Print `body.stl` and `lid.stl` with the flat side down. No supports are needed except for the groove in the lid:
add a support enforcer there in your slicer. The two screw holes are printed lying down without support: they
droop slightly, which is fine for M2 screws.

Print `mid.stl` with the display side down and with supports. That is the side with the round opening for the button.

## Assembly

The photos show the case in various stages of assembly.

### 1. The carrier with the Nano

The Nano sits on the back of the carrier part, the USB connector at the end, the button and the display on the front.
Wire everything first ({doc}`wiring`), then fit it into the case.

Keep the cables short. There is a channel through the carrier for the display's and the button's wires, but there is little
room for spare wire.

```{image} /_static/device/mid_part_assembled_bottom.jpg
:alt: The carrier part seen from below with the Nano and the button's contacts, next to the empty body and lid.
:align: center
```

### 2. The display and the button on the front

The display and the button are fitted from the front of the carrier. The red part is the button's cap.

```{image} /_static/device/mid_part_assembled_top.jpg
:alt: The carrier part seen from above with the display and the red button fitted, next to the body.
:align: center
```

Ideally the button, the Nano and the display sit firmly enough in the carrier that you can drop the carrier, display
side down, into the body and everything stays in place. If not, use a drop of hot glue.


### 3. Closing the case

```{image} /_static/device/lid_open.jpg
:alt: The case open: the lid with its two round mounting posts next to the body holding the Nano and the button's contacts.
:align: center
```

The lid has two screw holes and a few strategically placed posts that press the assembly together: the lid presses
down the row of solder joints of the Nano and the back of the button.

If the posts are a tiny bit too big, shave them with a sharp knife.

Finally put in the two screws, one from each side. Do not overtighten them: the thread is tiny and the plastic is
soft. They only need to hold the lid in place.

## Using the case with the firmware

The device can be used in both orientations:

- Button, display, USB (USB on the right): the default.
- USB, display, button (USB on the left): the display is upside down in this orientation and needs
  `trustctl config set rotate=on`.

Depending on where the cable comes from and on whether you are left- or right-handed, you may prefer one or the other.
You can try the rotation first on the calibration firmware with `trustctl calibrate rotate on` (see
{doc}`../setup/calibration`).

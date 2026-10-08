# The tray icon

`trans-windowd` is a background program with no window of its own: it waits for
the chords and opens whichever window they name. The tray icon is how it can
also be asked for the few things a background program has to offer without a
trip to the settings file.

Right-click the icon in the notification area.

| Item | What it does |
| --- | --- |
| Open panel | The same as pressing the panel chord: a panel over the window in front, or a panel split off beside that pane when `TRANS_PANEL_HOST=terminal` says so. |
| Close panel | Asks every panel window to close. There is no panel process to toggle — one is spawned for each press and ends with its window — so this is what "close" means here. |
| Settings… | The settings window, over the pane in front. |
| Reload settings | Reads the settings again and claims the chords anew, so a chord changed in the settings window takes effect without restarting the daemon. |
| Start at logon | Writes (or removes) the `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` entry that starts this program when you log on. The tick shows whether the entry is there and points at this program. |
| Quit | Gives the chords back, takes the icon away, and ends the daemon. |

The item is left out entirely when this copy of the program has no stable path
to put in the registry — a daemon started out of a build or temporary directory
would be gone by the next logon, and an entry pointing at a file that is not
there is worse than no entry.

## Turning it off

```
TRANS_TRAY=0
```

starts no tray at all. The chords work exactly as they always did; the only way
to end the program is to end the process. A tray that cannot be drawn — no
notification area, a shell that refuses the icon — is logged and otherwise
ignored: the chords keep working either way.

## Why the chords survive a reload

Windows hands a hotkey to the thread that claimed it, and only that thread can
give it back. The tray runs a message loop of its own — a hidden window on its
own thread, which is what a notification icon needs anyway — so it cannot take
the chords over. It asks the daemon's thread to do the work by posting a
message to it, and the daemon's loop answers by reading the settings and
claiming the chords again. The two loops meet nowhere else.

Nothing about the tray is on the path of a chord: a tray that has died leaves
the chords working.

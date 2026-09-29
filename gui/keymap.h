#pragma once

#include <QtGlobal>

// Maps a Linux evdev key code (what Qt reports as nativeScanCode() - 8 on
// Wayland and X11) to QEMU's "qnum": the PC/XT set 1 scancode, with
// 0xe0-prefixed extended keys folded into the high bit. Sending physical
// key positions instead of characters keeps the guest's own keyboard
// layout in charge, so dead keys, AltGr and non-US layouts work as they
// would on real hardware. Returns 0 for keys with no PC equivalent.
inline quint32 evdevToQnum(quint32 evdev) {
  // KEY_ESC (1) through KEY_KPDOT (83) are numbered exactly like set 1.
  if (evdev >= 1 && evdev <= 83)
    return evdev;
  switch (evdev) {
  case 85: // KEY_ZENKAKUHANKAKU
    return 0x76;
  case 86: // KEY_102ND (ISO key between left Shift and Z)
    return 0x56;
  case 87: // KEY_F11
    return 0x57;
  case 88: // KEY_F12
    return 0x58;
  case 89: // KEY_RO
    return 0x73;
  case 92: // KEY_HENKAN
    return 0x79;
  case 93: // KEY_KATAKANAHIRAGANA
    return 0x70;
  case 94: // KEY_MUHENKAN
    return 0x7b;
  case 96: // KEY_KPENTER
    return 0x9c;
  case 97: // KEY_RIGHTCTRL
    return 0x9d;
  case 98: // KEY_KPSLASH
    return 0xb5;
  case 99: // KEY_SYSRQ (Print Screen)
    return 0x54;
  case 100: // KEY_RIGHTALT (AltGr)
    return 0xb8;
  case 102: // KEY_HOME
    return 0xc7;
  case 103: // KEY_UP
    return 0xc8;
  case 104: // KEY_PAGEUP
    return 0xc9;
  case 105: // KEY_LEFT
    return 0xcb;
  case 106: // KEY_RIGHT
    return 0xcd;
  case 107: // KEY_END
    return 0xcf;
  case 108: // KEY_DOWN
    return 0xd0;
  case 109: // KEY_PAGEDOWN
    return 0xd1;
  case 110: // KEY_INSERT
    return 0xd2;
  case 111: // KEY_DELETE
    return 0xd3;
  case 113: // KEY_MUTE
    return 0xa0;
  case 114: // KEY_VOLUMEDOWN
    return 0xae;
  case 115: // KEY_VOLUMEUP
    return 0xb0;
  case 116: // KEY_POWER
    return 0xde;
  case 117: // KEY_KPEQUAL
    return 0x59;
  case 119: // KEY_PAUSE
    return 0xc6;
  case 121: // KEY_KPCOMMA
    return 0x7e;
  case 122: // KEY_HANGEUL
    return 0xf2;
  case 123: // KEY_HANJA
    return 0xf1;
  case 124: // KEY_YEN
    return 0x7d;
  case 125: // KEY_LEFTMETA
    return 0xdb;
  case 126: // KEY_RIGHTMETA
    return 0xdc;
  case 127: // KEY_COMPOSE (Menu)
    return 0xdd;
  default:
    return 0;
  }
}

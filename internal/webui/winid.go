package webui

// winID is one native window handle, as the surfaces above the placement layer
// see it.
//
// The surfaces never look inside it: they get one from xidOf and hand it straight
// back to a method on x11, so the only code that knows what the number MEANS is
// the backend that minted it. That is what lets the backend be swapped without
// touching a single surface, and it is why this type exists instead of the X11 one
// it used to be.
//
// uintptr rather than uint32 for one concrete reason: an X11 window id is 32 bits,
// a Windows HWND is a pointer, and a type that fits only the first would have to
// be found and widened by somebody working out why a handle was truncated. 0 means
// "no window", on every backend, and every caller already tests for it that way.
type winID uintptr

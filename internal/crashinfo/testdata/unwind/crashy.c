// A DLL whose functions call each other through frames of several kinds, then read a null pointer:
// framed (frame pointer, alloca) -> outer (saved registers) -> big (large stack allocation) -> leaf_read (no frame).
#include <stddef.h>
__declspec(noinline) __declspec(dllexport) int leaf_read(volatile int *p) { return *p; }
__declspec(noinline) int big(volatile int *p, int n) {
    volatile char buf[5000];
    buf[n & 4095] = (char)n;
    return leaf_read(p) + buf[7];
}
__declspec(noinline) __declspec(dllexport) int outer(int n) {
    volatile long long a = n, b = n * 2, c = n * 3;
    int r = big(NULL, (int)(a + b + c));
    return r + (int)a + (int)b;
}
__declspec(noinline) __declspec(dllexport) int framed(int n) {
    volatile char *mem = __builtin_alloca(n);
    mem[0] = 1;
    return outer(n) + mem[0];
}
__attribute__((naked)) void __chkstk(void) { __asm__("ret"); }

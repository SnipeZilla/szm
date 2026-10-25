import json, struct, sys
from unicorn import Uc, UcError, UC_ARCH_X86, UC_MODE_64, UC_HOOK_MEM_UNMAPPED
from unicorn.x86_const import *

dll = open(sys.argv[1], 'rb').read()
pe = struct.unpack_from('<I', dll, 0x3C)[0]
nsec = struct.unpack_from('<H', dll, pe + 6)[0]
optsize = struct.unpack_from('<H', dll, pe + 20)[0]
stamp = struct.unpack_from('<I', dll, pe + 8)[0]
opt = pe + 24
base = struct.unpack_from('<Q', dll, opt + 24)[0]
size_image = struct.unpack_from('<I', dll, opt + 56)[0]
uc = Uc(UC_ARCH_X86, UC_MODE_64)
uc.mem_map(base, (size_image + 0xFFF) & ~0xFFF)
uc.mem_write(base, dll[:0x400])
sec = opt + optsize
for k in range(nsec):
    s = sec + 40 * k
    vsize, va, rawsize, rawptr = struct.unpack_from('<IIII', dll, s + 8)
    uc.mem_write(base + va, dll[rawptr:rawptr + rawsize])
# exports: framed
def export(name):
    exp_rva = struct.unpack_from('<I', dll, opt + 112)[0]
    def rd(rva, n):
        return bytes(uc.mem_read(base + rva, n))
    h = rd(exp_rva, 40)
    nnames, funcs, names, ords = struct.unpack_from('<IIII', h, 24)
    for i in range(nnames):
        nr = struct.unpack('<I', rd(names + 4 * i, 4))[0]
        n = rd(nr, 64).split(b'\0')[0].decode()
        if n == name:
            o = struct.unpack('<H', rd(ords + 2 * i, 2))[0]
            return struct.unpack('<I', rd(funcs + 4 * o, 4))[0]
STACK, SSIZE = 0x7FF00000, 0x40000
uc.mem_map(STACK, SSIZE)
rsp = STACK + SSIZE - 0x100
SENTINEL = 0x7FFE0000  # the thread's start returns there
uc.mem_write(rsp, struct.pack('<Q', 0))  # return address 0: the thread's start
uc.reg_write(UC_X86_REG_RSP, rsp)
uc.reg_write(UC_X86_REG_RCX, 64)
for r in (UC_X86_REG_RBX, UC_X86_REG_RBP, UC_X86_REG_RSI, UC_X86_REG_RDI, UC_X86_REG_R12, UC_X86_REG_R13, UC_X86_REG_R14, UC_X86_REG_R15):
    uc.reg_write(r, 0x1111000000 + r)
fault = {}
def hook(uc, access, addr, size, value, user):
    fault['addr'] = addr
    return False
uc.hook_add(UC_HOOK_MEM_UNMAPPED, hook)
try:
    uc.emu_start(base + export('framed'), 0)
except UcError as e:
    pass
names = ['rax','rcx','rdx','rbx','rsp','rbp','rsi','rdi','r8','r9','r10','r11','r12','r13','r14','r15']
ids = [UC_X86_REG_RAX,UC_X86_REG_RCX,UC_X86_REG_RDX,UC_X86_REG_RBX,UC_X86_REG_RSP,UC_X86_REG_RBP,UC_X86_REG_RSI,UC_X86_REG_RDI,
       UC_X86_REG_R8,UC_X86_REG_R9,UC_X86_REG_R10,UC_X86_REG_R11,UC_X86_REG_R12,UC_X86_REG_R13,UC_X86_REG_R14,UC_X86_REG_R15]
regs = [uc.reg_read(i) for i in ids]
rip = uc.reg_read(UC_X86_REG_RIP)
sp = regs[4]
lo = sp & ~0xFFF
stack = bytes(uc.mem_read(lo, STACK + SSIZE - lo))
json.dump({'base': base, 'size': size_image, 'stamp': stamp, 'rip': rip, 'regs': regs, 'fault': fault.get('addr', -1),
           'stack_start': lo, 'stack': stack.hex()}, open(sys.argv[2], 'w'))
print('crashed at rip 0x%x (rva 0x%x), reading 0x%x, rsp 0x%x' % (rip, rip - base, fault.get('addr', -1), sp))

//go:build windows && amd64

#include "textflag.h"

TEXT ·cpuID(SB), NOSPLIT, $0-24
	MOVL eax+0(FP), AX
	MOVL ecx+4(FP), CX
	CPUID
	MOVL AX, a+8(FP)
	MOVL BX, b+12(FP)
	MOVL CX, c+16(FP)
	MOVL DX, d+20(FP)
	RET

TEXT ·xgetbv(SB), NOSPLIT, $0-8
	XORL CX, CX
	BYTE $0x0f
	BYTE $0x01
	BYTE $0xd0
	MOVL AX, a+0(FP)
	MOVL DX, d+4(FP)
	RET

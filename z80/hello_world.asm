; Hello world for the Z80

_start:
	; Write the message
	ld a, 3
	ld bc msg
	ld de, 14
	halt

	; Stop the machine
	ld a, 1
	halt

msg:
#Hello, World!%0a

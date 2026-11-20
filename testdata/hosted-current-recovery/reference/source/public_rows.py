"""Independent binary layout examples only, never valid wallet/capture authority."""
import struct
from common import integer, need

IMPORTED=2**31-1
ROW_CAP=65536

def le32(n):return struct.pack('<I',integer(n))

def account_row(pub,private,external,internal,name=b'default'):
    # Inert public fixture sentinel spans; neither key encryption nor derivation.
    body=le32(len(pub))+pub+le32(len(private))+private+le32(external)+le32(internal)+le32(len(name))+name
    return b'\x00'+le32(len(body))+body


def cursors(raw,account):
    need(type(raw) is bytes and 5<=len(raw)<=ROW_CAP)
    need(raw[0]==0 and int.from_bytes(raw[1:5],'little')==len(raw)-5)
    need(account in (0,IMPORTED))
    pos=5
    def skip(reserve):
        nonlocal pos
        need(pos+4<=len(raw));size=int.from_bytes(raw[pos:pos+4],'little');pos+=4
        need(size<=len(raw)-pos-reserve)
        pos+=size
        return size
    pub=skip(16);private=skip(12)
    # No slice/copy of either encrypted-key span is made by this parser.
    external=int.from_bytes(raw[pos:pos+4],'little');internal=int.from_bytes(raw[pos+4:pos+8],'little');pos+=8
    skip(0);need(pos==len(raw))
    if account==IMPORTED:need(pub==0 and private==0 and external==0 and internal==0)
    else:need(pub>0 and private>0 and external<=4096 and internal<=4096)
    return dict(account=account,next_external=external,next_internal=internal)

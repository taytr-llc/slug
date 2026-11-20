"""Conservative encoded compositions, independent of actual Go allocations."""
def compositions(caps):
    # These sum accepted component ceilings plus separately bounded syntax and
    # maximal scalar overhead. They are not a measured total runtime heap cap.
    rows=[
        ('stable_item',69632+4096+2048+8192+6144,90112),
        ('stable_page',8*90112+24576+8192,753664),
        ('entry_member',2048+2048+1536+512,6144),
        ('entry_receipt',5*6144+4096+2048+4096,49152),
        ('event_participant',53248+4096+5*6144+5*1024+8192,106496),
        ('event_snapshot',2*106496+8192+2048+4096,229376),
        ('event_receipt',229376+2*2048+4096,237568),
        ('seed_card_source',237568+5120+2048+2048+4096+16384,278528),
        ('card_record',278528+16384+8192,327680),
        ('bout_record',196608+28672+16384+16384,278528),
        ('history_retained',416*1024+32768+8192,512*1024),
    ]
    out=[]
    for label,n,cap in rows:
        if n>cap:raise ValueError('composition exceeds cap '+label)
        out.append(dict(name=label,composed_bytes=n,limit_bytes=cap,remaining_bytes=cap-n,claim='encoded cap composition only; not total Go heap'))
    return out

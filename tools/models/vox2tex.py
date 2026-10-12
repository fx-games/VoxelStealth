# MagicaVoxel block .vox -> block face textures, scaled up to 16×16 PNGs:
#   python3 tools/models/vox2tex.py in.vox outprefix
# writes outprefix_top.png, outprefix_bottom.png and outprefix_side.png (the
# -y face, top row up), each over the voxels' bounding box. Empty cells (a
# round trunk's corners) take the face's most common colour.
import struct, sys
from collections import Counter
from PIL import Image

b = open(sys.argv[1], 'rb').read()
vox, pal, i = {}, None, 8
while i < len(b):
    cid, n, kids = b[i:i + 4], *struct.unpack_from('<II', b, i + 4)
    d = b[i + 12:i + 12 + n]
    if cid == b'SIZE':
        sx, sy, sz = struct.unpack_from('<3I', d)
    elif cid == b'XYZI':
        for v in range(struct.unpack_from('<I', d)[0]):
            x, y, z, c = d[4 + v * 4:8 + v * 4]
            vox[x, y, z] = c
    elif cid == b'RGBA':
        pal = [tuple(d[c * 4:c * 4 + 4]) for c in range(256)]
    i += 12 + (0 if cid == b'MAIN' else n)
lo = [min(p[a] for p in vox) for a in range(3)]
nx, ny, nz = [max(p[a] for p in vox) - lo[a] + 1 for a in range(3)]
vox = {(x - lo[0], y - lo[1], z - lo[2]): c for (x, y, z), c in vox.items()}

def face(w, h, cells):  # cells(u, v) -> voxels from the surface inward
    cs = [[next((vox[p] for p in cells(u, v) if p in vox), 0) for v in range(h)] for u in range(w)]
    bg = Counter(c for col in cs for c in col if c).most_common(1)[0][0]
    img = Image.new('RGBA', (w, h))
    for u in range(w):
        for v in range(h):
            img.putpixel((u, v), pal[(cs[u][v] or bg) - 1])
    return img.resize((16, 16), Image.NEAREST)

face(nx, ny, lambda u, v: [(u, ny - 1 - v, z) for z in range(nz - 1, -1, -1)]).save(sys.argv[2] + '_top.png')
face(nx, ny, lambda u, v: [(u, v, z) for z in range(nz)]).save(sys.argv[2] + '_bottom.png')
face(nx, nz, lambda u, v: [(u, y, nz - 1 - v) for y in range(ny)]).save(sys.argv[2] + '_side.png')

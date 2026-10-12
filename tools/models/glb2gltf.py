# MagicaVoxel .glb export -> .gltf with embedded buffers:
#   python3 tools/models/glb2gltf.py in.glb out.gltf [units-per-block]
# Positions are baked to blocks (default 8 units, the Kyrise packs' 16 voxels),
# feet at y=0 and centred on x/z; the node transform must be identity.
import base64, json, struct, sys

src, dst = sys.argv[1], sys.argv[2]
k = 1 / float(sys.argv[3] if len(sys.argv) > 3 else 8)
b = open(src, 'rb').read()
n = struct.unpack_from('<I', b, 12)[0]
j = json.loads(b[20:20 + n])
binary = bytearray(b[20 + n + 8:])
assert all(not nd.get('matrix') and nd.get('scale', [1] * 3) == [1] * 3 and nd.get('translation', [0] * 3) == [0] * 3 for nd in j['nodes'])
pos = {p['attributes']['POSITION'] for m in j['meshes'] for p in m['primitives']}
acc = [j['accessors'][i] for i in pos]
lo = [min(a['min'][i] for a in acc) for i in range(3)]
hi = [max(a['max'][i] for a in acc) for i in range(3)]
off = [-(lo[0] + hi[0]) / 2, -lo[1], -(lo[2] + hi[2]) / 2]
for a in acc:
    bv = j['bufferViews'][a['bufferView']]
    stride, start = bv.get('byteStride', 12), bv.get('byteOffset', 0) + a.get('byteOffset', 0)
    for v in range(a['count']):
        o = start + v * stride
        p = struct.unpack_from('<3f', binary, o)
        struct.pack_into('<3f', binary, o, *[(p[i] + off[i]) * k for i in range(3)])
    a['min'] = [(a['min'][i] + off[i]) * k for i in range(3)]
    a['max'] = [(a['max'][i] + off[i]) * k for i in range(3)]
j['buffers'] = [{'byteLength': len(binary), 'uri': 'data:application/octet-stream;base64,' + base64.b64encode(binary).decode()}]
json.dump(j, open(dst, 'w'))

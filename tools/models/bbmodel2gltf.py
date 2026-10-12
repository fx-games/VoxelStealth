# Blockbench .bbmodel (cubes, one embedded texture) -> glTF:
#   python3 tools/models/bbmodel2gltf.py in.bbmodel out.gltf [px-per-block]
# Blockbench pixels become blocks at px-per-block (default 16), keeping the
# model's origin; one mesh, the texture embedded as a PNG. Faces without a
# texture are dropped.
import base64, json, math, struct, sys

src, dst = sys.argv[1], sys.argv[2]
scale = 1 / float(sys.argv[3] if len(sys.argv) > 3 else 16)
m = json.load(open(src))
assert len(m['textures']) == 1, 'one texture only'
tex = m['textures'][0]
uw, uh = tex.get('uv_width', m['resolution']['width']), tex.get('uv_height', m['resolution']['height'])

def rotate(p, rot, origin):
    # ponytail: one rotation axis per cube (all Blockbench's Java format allows); add Euler order when a model needs more
    assert sum(1 for a in rot if a) <= 1, 'multi-axis cube rotation'
    p = [p[i] - origin[i] for i in range(3)]
    for axis, deg in enumerate(rot):
        if not deg: continue
        c, s = math.cos(math.radians(deg)), math.sin(math.radians(deg))
        i, j = [(1, 2), (2, 0), (0, 1)][axis]
        p[i], p[j] = c * p[i] - s * p[j], s * p[i] + c * p[j]
    return [p[i] + origin[i] for i in range(3)]

pos, nor, uv = [], [], []
for e in m['elements']:
    assert e.get('type', 'cube') == 'cube', e.get('type')
    g = e.get('inflate', 0)
    (x1, y1, z1), (x2, y2, z2) = [a - g for a in e['from']], [a + g for a in e['to']]
    # corners of each face seen from outside: top-left, top-right, bottom-right, bottom-left
    corners = {
        'north': [(x2, y2, z1), (x1, y2, z1), (x1, y1, z1), (x2, y1, z1)],
        'south': [(x1, y2, z2), (x2, y2, z2), (x2, y1, z2), (x1, y1, z2)],
        'east': [(x2, y2, z2), (x2, y2, z1), (x2, y1, z1), (x2, y1, z2)],
        'west': [(x1, y2, z1), (x1, y2, z2), (x1, y1, z2), (x1, y1, z1)],
        'up': [(x1, y2, z1), (x2, y2, z1), (x2, y2, z2), (x1, y2, z2)],
        'down': [(x1, y1, z2), (x2, y1, z2), (x2, y1, z1), (x1, y1, z1)],
    }
    for name, f in e['faces'].items():
        if f.get('texture') is None: continue
        assert not f.get('rotation'), 'face UV rotation'
        q = [[c * scale for c in rotate(list(p), e.get('rotation') or [0, 0, 0], e.get('origin', [0, 0, 0]))] for p in corners[name]]
        u1, v1, u2, v2 = f['uv']
        t = [(u1, v1), (u2, v1), (u2, v2), (u1, v2)]
        a, b = [q[3][i] - q[0][i] for i in range(3)], [q[2][i] - q[0][i] for i in range(3)]
        n = [a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2], a[0]*b[1]-a[1]*b[0]]
        l = sum(x * x for x in n) ** 0.5
        for k in (0, 3, 2, 0, 2, 1):  # counter-clockwise from outside
            pos.append(q[k]); nor.append([x / l for x in n]); uv.append([t[k][0] / uw, t[k][1] / uh])

k = len(pos)
flat = lambda a: [x for r in a for x in r]
blobs = [struct.pack('<%df' % (3*k), *flat(pos)), struct.pack('<%df' % (3*k), *flat(nor)),
         struct.pack('<%df' % (2*k), *flat(uv)), struct.pack('<%dI' % k, *range(k))]
views, off = [], 0
for b in blobs:
    views.append({'buffer': 0, 'byteOffset': off, 'byteLength': len(b)}); off += len(b)
buf = b''.join(blobs)
json.dump({
    'asset': {'version': '2.0', 'generator': 'bbmodel2gltf (VoxelStealth)'},
    'scene': 0, 'scenes': [{'nodes': [0]}], 'nodes': [{'name': m.get('name', 'model'), 'mesh': 0}],
    'meshes': [{'primitives': [{'attributes': {'POSITION': 0, 'NORMAL': 1, 'TEXCOORD_0': 2}, 'indices': 3, 'material': 0}]}],
    'materials': [{'pbrMetallicRoughness': {'baseColorTexture': {'index': 0}}, 'alphaMode': 'MASK'}],
    'textures': [{'source': 0, 'sampler': 0}], 'samplers': [{'magFilter': 9728, 'minFilter': 9728}],
    'images': [{'uri': tex['source']}],
    'accessors': [
        {'bufferView': 0, 'componentType': 5126, 'count': k, 'type': 'VEC3',
         'min': [min(p[i] for p in pos) for i in range(3)], 'max': [max(p[i] for p in pos) for i in range(3)]},
        {'bufferView': 1, 'componentType': 5126, 'count': k, 'type': 'VEC3'},
        {'bufferView': 2, 'componentType': 5126, 'count': k, 'type': 'VEC2'},
        {'bufferView': 3, 'componentType': 5125, 'count': k, 'type': 'SCALAR'}],
    'bufferViews': views,
    'buffers': [{'byteLength': len(buf), 'uri': 'data:application/octet-stream;base64,' + base64.b64encode(buf).decode()}],
}, open(dst, 'w'), indent=1)

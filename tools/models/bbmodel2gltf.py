# Blockbench .bbmodel (cubes, embedded textures) -> glTF:
#   python3 tools/models/bbmodel2gltf.py in.bbmodel out.gltf [px-per-block]
# Blockbench pixels become blocks at px-per-block (default 16), keeping the
# model's origin; group rotations are baked in. One mesh with a primitive
# per texture, each embedded as a PNG. Faces without a texture are dropped.
import base64, json, math, struct, sys

src, dst = sys.argv[1], sys.argv[2]
scale = 1 / float(sys.argv[3] if len(sys.argv) > 3 else 16)
m = json.load(open(src))
texs = m['textures']
uvsize = [(t.get('uv_width', m['resolution']['width']), t.get('uv_height', m['resolution']['height'])) for t in texs]

# Each element's enclosing groups, innermost first.
groups = {g['uuid']: g for g in m.get('groups', [])}
parents = {}
def walk(nodes, chain):
    for n in nodes:
        if isinstance(n, str):
            parents[n] = chain
        else:
            walk(n.get('children', []), [groups.get(n['uuid'], n)] + chain)
walk(m.get('outliner', []), [])

def rotate(p, rot, origin):
    # Blockbench's Euler order is ZYX: X is applied first.
    p = [p[i] - origin[i] for i in range(3)]
    for axis, deg in enumerate(rot):
        if not deg: continue
        c, s = math.cos(math.radians(deg)), math.sin(math.radians(deg))
        i, j = [(1, 2), (2, 0), (0, 1)][axis]
        p[i], p[j] = c * p[i] - s * p[j], s * p[i] + c * p[j]
    return [p[i] + origin[i] for i in range(3)]

prims = [([], [], []) for _ in texs]  # per texture: positions, normals, UVs
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
        q = []
        for p in corners[name]:
            p = rotate(list(p), e.get('rotation') or [0, 0, 0], e.get('origin', [0, 0, 0]))
            for g in parents.get(e['uuid'], []):
                p = rotate(p, g.get('rotation') or [0, 0, 0], g.get('origin', [0, 0, 0]))
            q.append([c * scale for c in p])
        pos, nor, uv = prims[f['texture']]
        uw, uh = uvsize[f['texture']]
        u1, v1, u2, v2 = f['uv']
        t = [(u1, v1), (u2, v1), (u2, v2), (u1, v2)]
        a, b = [q[3][i] - q[0][i] for i in range(3)], [q[2][i] - q[0][i] for i in range(3)]
        n = [a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2], a[0]*b[1]-a[1]*b[0]]
        l = sum(x * x for x in n) ** 0.5
        if not l: continue  # the edge of a flat cube
        for k in (0, 3, 2, 0, 2, 1):  # counter-clockwise from outside
            pos.append(q[k]); nor.append([x / l for x in n]); uv.append([t[k][0] / uw, t[k][1] / uh])

flat = lambda a: [x for r in a for x in r]
blobs, accs, out = [], [], []
def add(b, count, typ, ctype=5126, **mm):
    blobs.append(b)
    accs.append({'bufferView': len(blobs) - 1, 'componentType': ctype, 'count': count, 'type': typ, **mm})
    return len(accs) - 1
for t, (pos, nor, uv) in enumerate(prims):
    k = len(pos)
    if not k: continue
    out.append({'attributes': {
        'POSITION': add(struct.pack('<%df' % (3*k), *flat(pos)), k, 'VEC3',
                        min=[min(p[i] for p in pos) for i in range(3)], max=[max(p[i] for p in pos) for i in range(3)]),
        'NORMAL': add(struct.pack('<%df' % (3*k), *flat(nor)), k, 'VEC3'),
        'TEXCOORD_0': add(struct.pack('<%df' % (2*k), *flat(uv)), k, 'VEC2')},
        'indices': add(struct.pack('<%dI' % k, *range(k)), k, 'SCALAR', 5125), 'material': t})
views, off = [], 0
for b in blobs:
    views.append({'buffer': 0, 'byteOffset': off, 'byteLength': len(b)}); off += len(b)
buf = b''.join(blobs)
json.dump({
    'asset': {'version': '2.0', 'generator': 'bbmodel2gltf (VoxelStealth)'},
    'scene': 0, 'scenes': [{'nodes': [0]}], 'nodes': [{'name': m.get('name', 'model'), 'mesh': 0}],
    'meshes': [{'primitives': out}],
    'materials': [{'pbrMetallicRoughness': {'baseColorTexture': {'index': t}}, 'alphaMode': 'MASK'} for t in range(len(texs))],
    'textures': [{'source': t, 'sampler': 0} for t in range(len(texs))], 'samplers': [{'magFilter': 9728, 'minFilter': 9728}],
    'images': [{'uri': t['source']} for t in texs],
    'accessors': accs, 'bufferViews': views,
    'buffers': [{'byteLength': len(buf), 'uri': 'data:application/octet-stream;base64,' + base64.b64encode(buf).decode()}],
}, open(dst, 'w'), indent=1)

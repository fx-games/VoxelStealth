# Watoosh's Blockbench OBJ lamp post -> assets/models/lamp.gltf:
#   python3 tools/models/lamp2gltf.py model.obj assets/models/lamp.gltf
# Block units at 1/32 (1.625 tall), origin at the foot of the pole, one
# named node per cube, untextured: UVs point at u=0.25 (iron) or u=0.75
# (the light cube) of a 2x1 texture painted in code.
import base64, json, struct, sys

src, dst = sys.argv[1], sys.argv[2]
V, objs = [], []
for line in open(src):
    t = line.split()
    if not t: continue
    if t[0] == 'v': V.append([float(x) for x in t[1:4]])
    elif t[0] == 'o': objs.append([])
    elif t[0] == 'f': objs[-1].append([int(x.split('/')[0]) - 1 for x in t[1:]])
cx = (min(v[0] for v in V) + max(v[0] for v in V)) / 2
cz = (min(v[2] for v in V) + max(v[2] for v in V)) / 2
y0 = min(v[1] for v in V)
P = [[round((v[0] - cx) * 2) / 64, round((v[1] - y0) * 2) / 64, round((v[2] - cz) * 2) / 64] for v in V]  # snap to the 1/2 px grid
names = ['pole', 'plate', 'post1', 'post2', 'post3', 'post4', 'light', 'roof', 'top']
assert len(objs) == len(names), len(objs)

buf, views, accs, meshes, nodes = b'', [], [], [], []
def add(data, fmt, n, typ, target, **mm):
    global buf
    views.append({'buffer': 0, 'byteOffset': len(buf), 'byteLength': len(data), 'target': target})
    buf += data
    accs.append({'bufferView': len(views) - 1, 'componentType': 5126 if fmt == 'f' else 5123, 'count': n, 'type': typ, **mm})
    return len(accs) - 1
for name, faces in zip(names, objs):
    pos, nor, uv = [], [], []
    for f in faces:
        tris = [(f[0], f[i], f[i + 1]) for i in range(1, len(f) - 1)]
        for a, b, c in tris:
            pa, pb, pc = P[a], P[b], P[c]
            e1 = [pb[i] - pa[i] for i in range(3)]; e2 = [pc[i] - pa[i] for i in range(3)]
            n = [e1[1]*e2[2]-e1[2]*e2[1], e1[2]*e2[0]-e1[0]*e2[2], e1[0]*e2[1]-e1[1]*e2[0]]
            l = sum(x * x for x in n) ** 0.5 or 1
            for p in (pa, pb, pc):
                pos.append(p); nor.append([x / l for x in n]); uv.append([0.75 if name == 'light' else 0.25, 0.5])
    k = len(pos)
    flat = lambda a: [x for r in a for x in r]
    p = add(struct.pack('<%df' % (3*k), *flat(pos)), 'f', k, 'VEC3', 34962,
            min=[min(r[i] for r in pos) for i in range(3)], max=[max(r[i] for r in pos) for i in range(3)])
    nn = add(struct.pack('<%df' % (3*k), *flat(nor)), 'f', k, 'VEC3', 34962)
    t = add(struct.pack('<%df' % (2*k), *flat(uv)), 'f', k, 'VEC2', 34962)
    idx = struct.pack('<%dH' % k, *range(k)); idx += b'\0' * (len(idx) % 4)
    i = add(idx, 'H', k, 'SCALAR', 34963)
    meshes.append({'name': name, 'primitives': [{'attributes': {'POSITION': p, 'NORMAL': nn, 'TEXCOORD_0': t}, 'indices': i, 'material': 0}]})
    nodes.append({'name': name, 'mesh': len(meshes) - 1})
nodes.append({'name': 'lamp', 'children': list(range(len(names)))})
json.dump({
    'asset': {'version': '2.0', 'generator': 'obj2gltf (VoxelStealth)', 'extras': {
        'source': 'https://sketchfab.com/3d-models/minecraft-lamp-post-96db4410ed9d432492904a4fc5c57e8a',
        'author': 'Watoosh', 'license': 'CC BY-SA 4.0', 'note': 'scaled to 1/32 block; texture replaced by one painted in code'}},
    'scene': 0, 'scenes': [{'nodes': [len(nodes) - 1]}], 'nodes': nodes, 'meshes': meshes,
    'materials': [{'name': 'lamp', 'pbrMetallicRoughness': {'baseColorFactor': [1, 1, 1, 1]}}],
    'accessors': accs, 'bufferViews': views,
    'buffers': [{'byteLength': len(buf), 'uri': 'data:application/octet-stream;base64,' + base64.b64encode(buf).decode()}],
}, open(dst, 'w'), indent=1)

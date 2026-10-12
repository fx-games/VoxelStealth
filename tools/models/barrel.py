# Our barrel as a .bbmodel, for bbmodel2gltf.py:
#   python3 tools/models/barrel.py <rpg16pack textures/block dir> barrel.bbmodel
#   python3 tools/models/bbmodel2gltf.py barrel.bbmodel assets/models/barrel.gltf
import base64, json, sys
T = sys.argv[1]
texs = [{'source': 'data:image/png;base64,' + base64.b64encode(open(f'{T}/{n}.png', 'rb').read()).decode(), 'uv_width': 16, 'uv_height': 16}
        for n in ('spruce_log', 'spruce_planks', 'iron_block')]
els = []
def cube(r, y0, y1, side, ends=None):
    f = {d: {'uv': [8 - r, 16 - y1, 8 + r, 16 - y0], 'texture': side} for d in ('north', 'south', 'east', 'west')}
    if ends is not None:
        f['up'] = f['down'] = {'uv': [8 - r, 8 - r, 8 + r, 8 + r], 'texture': ends}
    els.append({'uuid': str(len(els)), 'from': [-r, y0, -r], 'to': [r, y1, r], 'faces': f})
cube(6, 0, 16, 0, 1)        # body with lids
cube(7, 2, 14, 0, 1)        # belly
for y in (1, 4, 11, 14):    # hoops
    cube(7 if 2 < y < 14 else 6.5, y, y + 1, 2)
    els[-1]['inflate'] = 0.1
json.dump({'name': 'barrel', 'resolution': {'width': 16, 'height': 16}, 'textures': texs, 'elements': els, 'outliner': []}, open(sys.argv[2], 'w'))

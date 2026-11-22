
# 阈值 + 红舰底整行水线 + 仅水平闭运算（补竖向白缝，不填三角桅的空隙）+ 连通域筛选
# 用法: fmask2.py src out thr x0 y0 x1 y1 [minFrac] [closeH]
import sys, numpy as np
from PIL import Image
from collections import deque
src,out = sys.argv[1], sys.argv[2]
thr=float(sys.argv[3]); x0,y0,x1,y1=[int(v) for v in sys.argv[4:8]]
minfrac=float(sys.argv[8]) if len(sys.argv)>8 else 0.02
clh=int(sys.argv[9]) if len(sys.argv)>9 else 2
im = Image.open(src).convert('RGB').crop((x0,y0,x1,y1))
a = np.asarray(im).astype(np.int32)
lum = 0.299*a[:,:,0]+0.587*a[:,:,1]+0.114*a[:,:,2]
h,w = lum.shape
r,g,b = a[:,:,0],a[:,:,1],a[:,:,2]
red=(r>85)&(r>g+35)&(r>b+35)
tops=[np.nonzero(red[:,i])[0][0] for i in range(w) if red[:,i].sum()>=3]
if tops and len(tops) >= w*0.15:
    cr=int(np.median(tops)); lum=lum.copy(); lum[cr:,:]=255.0
fg = lum < thr
def dilh(m,k):
    o=m.copy()
    for _ in range(k):
        p=np.pad(o,((0,0),(1,1)))
        o = p[:,:-2] | p[:,1:-1] | p[:,2:]
    return o
def eroh(m,k):
    o=m.copy()
    for _ in range(k):
        p=np.pad(o,((0,0),(1,1)))
        o = p[:,:-2] & p[:,1:-1] & p[:,2:]
    return o
if clh: fg = eroh(dilh(fg,clh),clh)
seen=np.zeros((h,w),bool); comps=[]
for sy,sx in zip(*np.nonzero(fg)):
    if seen[sy,sx]: continue
    comp=[]; dq=deque([(sy,sx)]); seen[sy,sx]=True
    while dq:
        y,x=dq.popleft(); comp.append((y,x))
        for dy,dx in ((1,0),(-1,0),(0,1),(0,-1)):
            ny,nx=y+dy,x+dx
            if 0<=ny<h and 0<=nx<w and fg[ny,nx] and not seen[ny,nx]:
                seen[ny,nx]=True; dq.append((ny,nx))
    ys=[c[0] for c in comp]; xs=[c[1] for c in comp]
    comps.append((len(comp),(min(ys),max(ys),min(xs),max(xs)),comp))
comps.sort(key=lambda t:-t[0]); main=comps[0]; mtop=main[1][0]
mw=max(4,int(w*minfrac)); keep=np.zeros((h,w),bool)
for n,bb,comp in comps:
    if n!=main[0] and bb[1] < mtop: continue
    if (bb[3]-bb[2]+1) < mw and n < mw*2: continue
    for y,x in comp: keep[y,x]=True
ys,xs=np.nonzero(keep)
sub=keep[ys.min():ys.max()+1, xs.min():xs.max()+1]
sh,sw=sub.shape
rgba=np.zeros((sh,sw,4),np.uint8); rgba[:,:,3]=(sub*255).astype(np.uint8)
Image.fromarray(rgba,'RGBA').save(out)
print(out, sw, sh)

#!/usr/bin/env python3
"""Generate synthetic SUB/RAW frames to self-test the probe pipeline."""
import os, numpy as np
from PIL import Image, ImageDraw, ImageFont

OUTROOT = os.environ.get('SUBPROBE_OUT', '/tmp/subprobe-out')  # never the repo
FRM = os.path.join(OUTROOT, 'frames')
W, H = 1280, 720
FONT = '/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf'

EN = ["You really think you can win?", "I won't let you hurt them.",
      "We have to move, now!", "This is the end of the road.",
      "Don't underestimate me.", "The mission starts tomorrow."]
RU = ["Ты правда думаешь, что победишь?", "Я не позволю тебе их тронуть.",
      "Нам нужно уходить, немедленно!", "Это конец пути.",
      "Не недооценивай меня.", "Миссия начнётся завтра."]

def bg(seed):
    rng = np.random.default_rng(seed)
    # smooth-ish anime-like background: low-freq gradient + a few bright shapes + grain
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
    r = (0.5+0.5*np.sin(xx/300+seed))*120 + 30
    g = (0.5+0.5*np.cos(yy/250+seed))*120 + 40
    b = (0.5+0.5*np.sin((xx+yy)/400))*120 + 50
    img = np.stack([r, g, b], -1)
    img += rng.normal(0, 8, img.shape)            # grain
    for _ in range(6):                            # bright blobs (clouds/highlights)
        cx, cy, rad = rng.integers(0,W), rng.integers(0,H), rng.integers(20,90)
        m = ((xx-cx)**2+(yy-cy)**2) < rad**2
        img[m] = np.clip(img[m]+rng.integers(40,160), 0, 255)
    return Image.fromarray(np.clip(img,0,255).astype(np.uint8))

def draw_sub(img, text, size=46):
    d = ImageDraw.Draw(img)
    font = ImageFont.truetype(FONT, size)
    tb = d.textbbox((0,0), text, font=font)
    tw, th = tb[2]-tb[0], tb[3]-tb[1]
    x = (W-tw)//2; y = int(H*0.84)
    for dx in (-2,-1,0,1,2):                       # black outline
        for dy in (-2,-1,0,1,2):
            d.text((x+dx, y+dy), text, font=font, fill=(0,0,0))
    d.text((x, y), text, font=font, fill=(255,255,255))
    return img

def gen(kind, lines):
    out = os.path.join(FRM, kind); os.makedirs(out, exist_ok=True)
    for f in os.listdir(out):
        if f.endswith('.png'): os.remove(os.path.join(out,f))
    for i in range(8):
        img = bg(i*7+ (0 if kind=='raw' else 3))
        if kind != 'raw':
            # subs flicker: present in ~6/8 frames (dialogue on screen)
            if i not in (2,5):
                img = draw_sub(img, lines[i % len(lines)])
        img.save(os.path.join(out, f'f_{i:03d}.png'))
    print(f"wrote {kind} -> {out}")

gen('raw', None)
gen('sub_en', EN)
gen('sub_ru', RU)

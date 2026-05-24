from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


ROOT = Path(__file__).resolve().parent
FONT_DIR = Path("C:/Windows/Fonts")


def font(name: str, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(str(FONT_DIR / name), size=size)


def lerp(a: int, b: int, t: float) -> int:
    return round(a + (b - a) * t)


def mix(c1: tuple[int, int, int], c2: tuple[int, int, int], t: float) -> tuple[int, int, int]:
    return tuple(lerp(a, b, t) for a, b in zip(c1, c2))


def rounded_rectangle(
    draw: ImageDraw.ImageDraw,
    xy: tuple[int, int, int, int],
    radius: int,
    fill: tuple[int, int, int, int],
    outline: tuple[int, int, int, int] | None = None,
    width: int = 1,
) -> None:
    draw.rounded_rectangle(xy, radius=radius, fill=fill, outline=outline, width=width)


def text_size(draw: ImageDraw.ImageDraw, text: str, fnt: ImageFont.FreeTypeFont) -> tuple[int, int]:
    box = draw.textbbox((0, 0), text, font=fnt)
    return box[2] - box[0], box[3] - box[1]


def draw_centered_text(
    draw: ImageDraw.ImageDraw,
    box: tuple[int, int, int, int],
    text: str,
    fnt: ImageFont.FreeTypeFont,
    fill: tuple[int, int, int, int],
    spacing: int = 0,
) -> None:
    lines = text.split("\n")
    sizes = [text_size(draw, line, fnt) for line in lines]
    total_h = sum(h for _, h in sizes) + spacing * (len(lines) - 1)
    y = box[1] + ((box[3] - box[1]) - total_h) // 2
    for line, (w, h) in zip(lines, sizes):
        x = box[0] + ((box[2] - box[0]) - w) // 2
        draw.text((x, y), line, font=fnt, fill=fill)
        y += h + spacing


def make_background(w: int, h: int) -> Image.Image:
    img = Image.new("RGBA", (w, h))
    pixels = img.load()
    left = (5, 12, 26)
    mid = (8, 32, 44)
    right = (32, 27, 38)
    for x in range(w):
        t = x / max(1, w - 1)
        base = mix(left, mid, min(t / 0.46, 1)) if t < 0.46 else mix(mid, right, (t - 0.46) / 0.54)
        for y in range(h):
            shade = int(10 * (y / h))
            pixels[x, y] = (max(base[0] - shade, 0), max(base[1] - shade, 0), max(base[2] - shade, 0), 255)
    return img


def overlay_grid(draw: ImageDraw.ImageDraw, w: int, h: int) -> None:
    for x in range(0, w, 44):
        alpha = 18 if x < 930 else 12
        draw.line((x, 0, x, h), fill=(255, 255, 255, alpha), width=1)
    for y in range(0, h, 44):
        draw.line((0, y, w, y), fill=(255, 255, 255, 14), width=1)


def draw_glow_line(draw: ImageDraw.ImageDraw, start: tuple[int, int], end: tuple[int, int], color: tuple[int, int, int]) -> None:
    for width, alpha in [(10, 26), (5, 54), (2, 170)]:
        draw.line((*start, *end), fill=(*color, alpha), width=width)


def draw_badge(draw: ImageDraw.ImageDraw, x: int, y: int, label: str, fnt: ImageFont.FreeTypeFont) -> None:
    tw, th = text_size(draw, label, fnt)
    pad_x = 18
    pad_y = 11
    box = (x, y, x + tw + pad_x * 2 + 22, y + th + pad_y * 2 + 2)
    rounded_rectangle(draw, box, 24, (12, 28, 42, 245), (98, 214, 255, 86), 1)
    cy = (box[1] + box[3]) // 2
    draw.ellipse((x + 18, cy - 5, x + 28, cy + 5), fill=(89, 240, 154, 255))
    draw.text((x + 40, y + pad_y - 2), label, font=fnt, fill=(222, 242, 255, 255))


def draw_pill(draw: ImageDraw.ImageDraw, x: int, y: int, label: str, fnt: ImageFont.FreeTypeFont) -> int:
    tw, th = text_size(draw, label, fnt)
    box = (x, y, x + tw + 28, y + th + 22)
    rounded_rectangle(draw, box, 24, (14, 31, 45, 238), (32, 211, 181, 90), 1)
    draw.text((x + 14, y + 9), label, font=fnt, fill=(226, 245, 255, 255))
    return box[2]


def draw_terminal(draw: ImageDraw.ImageDraw) -> None:
    box = (80, 368, 430, 550)
    rounded_rectangle(draw, box, 22, (5, 13, 24, 170), (255, 255, 255, 38), 1)
    draw.line((box[0], box[1] + 42, box[2], box[1] + 42), fill=(255, 255, 255, 28), width=1)
    dots = [(255, 111, 97), (249, 178, 78), (89, 240, 154)]
    for i, c in enumerate(dots):
        x = box[0] + 18 + i * 19
        draw.ellipse((x, box[1] + 16, x + 10, box[1] + 26), fill=(*c, 255))

    mono = font("consola.ttf", 17)
    x = box[0] + 20
    y = box[1] + 60
    draw.text((x, y), "$", font=mono, fill=(98, 214, 255, 255))
    draw.text((x + 20, y), "connect gpt-relay", font=mono, fill=(220, 238, 255, 255))
    y += 38
    draw.text((x, y), "200 OK", font=mono, fill=(89, 240, 154, 255))
    draw.text((x + 74, y), " secure route ready", font=mono, fill=(220, 238, 255, 255))
    y += 38
    draw.text((x, y), "latency: optimized", font=mono, fill=(220, 238, 255, 255))


def draw_node(draw: ImageDraw.ImageDraw, xy: tuple[int, int, int, int], label: str, main: bool = False) -> None:
    if main:
        fill = (18, 52, 58, 226)
        outline = (32, 211, 181, 120)
        fnt = font("segoeuib.ttf", 20)
    else:
        fill = (12, 22, 34, 170)
        outline = (255, 255, 255, 45)
        fnt = font("segoeuib.ttf", 20)
    rounded_rectangle(draw, xy, 22, fill, outline, 1)
    draw_centered_text(draw, xy, label, fnt, (248, 250, 252, 255), spacing=2)


def draw_diagram(draw: ImageDraw.ImageDraw) -> None:
    origin_x = 505
    origin_y = 84
    api = (origin_x + 150, origin_y + 182, origin_x + 292, origin_y + 274)
    nodes = [
        ((origin_x + 8, origin_y + 62, origin_x + 126, origin_y + 136), "US"),
        ((origin_x + 314, origin_y + 48, origin_x + 432, origin_y + 122), "EU"),
        ((origin_x + 18, origin_y + 318, origin_x + 136, origin_y + 392), "Asia"),
        ((origin_x + 312, origin_y + 324, origin_x + 430, origin_y + 398), "SaaS"),
    ]
    api_center = ((api[0] + api[2]) // 2, (api[1] + api[3]) // 2)
    for xy, _ in nodes:
        center = ((xy[0] + xy[2]) // 2, (xy[1] + xy[3]) // 2)
        draw_glow_line(draw, center, api_center, (98, 214, 255))

    for xy, label in nodes:
        draw_node(draw, xy, label)
    draw_node(draw, api, "GPT\nRelay", main=True)
    draw.ellipse((api_center[0] - 17, api_center[1] - 17, api_center[0] + 17, api_center[1] + 17), outline=(89, 240, 154, 190), width=3)


def draw_brand(draw: ImageDraw.ImageDraw) -> None:
    x, y = 76, 56
    mark = Image.new("RGBA", (68, 68), (0, 0, 0, 0))
    mark_px = mark.load()
    for i in range(68):
        for j in range(68):
            t = (i + j) / 134
            c = mix((32, 211, 181), (98, 214, 255), min(t * 1.5, 1))
            if t > 0.55:
                c = mix(c, (249, 178, 78), (t - 0.55) / 0.45)
            mark_px[i, j] = (*c, 255)
    mask = Image.new("L", (68, 68), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, 68, 68), radius=18, fill=255)
    draw.bitmap((x, y), mask, fill=(32, 211, 181, 255))
    draw._image.alpha_composite(mark, (x, y))
    draw.rounded_rectangle((x, y, x + 68, y + 68), radius=18, outline=(255, 255, 255, 56), width=1)
    draw_centered_text(draw, (x, y, x + 68, y + 68), "API", font("segoeuib.ttf", 24), (6, 17, 30, 255))
    draw.text((x + 84, y + 4), "RELAY ACCESS", font=font("segoeuib.ttf", 22), fill=(255, 255, 255, 255))
    draw.text((x + 84, y + 38), "For global AI builders", font=font("segoeui.ttf", 15), fill=(182, 199, 214, 255))


def draw_copy(draw: ImageDraw.ImageDraw) -> None:
    x = 990
    y = 78
    draw_badge(draw, x, y, "Independent GPT API relay service", font("segoeuib.ttf", 19))
    draw.text((x, y + 78), "Reliable GPT", font=font("segoeuib.ttf", 64), fill=(255, 255, 255, 255))
    draw.text((x, y + 140), "API Relay", font=font("segoeuib.ttf", 64), fill=(32, 211, 181, 255))
    lead = "Fast access, stable routing, and simple\nintegration for developers and AI teams."
    draw.multiline_text((x, y + 235), lead, font=font("seguisb.ttf", 25), fill=(214, 228, 239, 255), spacing=6)

    labels = ["Global Delivery", "Low Latency", "Easy Setup", "24/7 Support"]
    px, py = x, y + 352
    pill_font = font("segoeuib.ttf", 16)
    for label in labels:
        tw, _ = text_size(draw, label, pill_font)
        pill_w = tw + 28
        if px + pill_w > 1580:
            px = x
            py += 54
        next_x = draw_pill(draw, px, py, label, pill_font) + 12
        px = next_x

    cta = (x, y + 452, x + 376, y + 512)
    rounded_rectangle(draw, cta, 18, (32, 211, 181, 255), None, 1)
    draw_centered_text(draw, cta, "DM FOR ACCESS", font("segoeuib.ttf", 22), (4, 16, 28, 255))
    draw.multiline_text(
        (x + 402, y + 458),
        "Setup guidance and\nflexible plans available.",
        font=font("segoeui.ttf", 16),
        fill=(182, 199, 214, 255),
        spacing=5,
    )


def draw_status_panel(draw: ImageDraw.ImageDraw) -> None:
    box = (78, 274, 392, 346)
    rounded_rectangle(draw, box, 24, (12, 22, 34, 210), (255, 255, 255, 40), 1)
    draw.text((104, 294), "Gateway Status", font=font("segoeuib.ttf", 20), fill=(255, 255, 255, 255))
    online = (270, 290, 368, 322)
    rounded_rectangle(draw, online, 16, (89, 240, 154, 36), None, 1)
    draw.ellipse((282, 302, 290, 310), fill=(89, 240, 154, 255))
    draw.text((298, 296), "ONLINE", font=font("segoeuib.ttf", 15), fill=(167, 255, 200, 255))


def render() -> None:
    w, h = 1640, 624
    img = make_background(w, h)
    draw = ImageDraw.Draw(img, "RGBA")

    overlay_grid(draw, w, h)
    draw.polygon([(0, 0), (430, 0), (310, h), (0, h)], fill=(1, 9, 20, 118))
    draw.polygon([(110, 0), (128, 0), (620, h), (598, h)], fill=(32, 211, 181, 30))
    draw.polygon([(690, 0), (705, 0), (1125, h), (1105, h)], fill=(249, 178, 78, 30))
    draw.rectangle((0, 0, w, h), outline=(255, 255, 255, 18), width=1)

    draw_brand(draw)
    draw_terminal(draw)
    draw_diagram(draw)
    draw_status_panel(draw)
    draw_copy(draw)

    hi = ROOT / "facebook-gpt-relay-banner-1640x624.png"
    std = ROOT / "facebook-gpt-relay-banner-820x312.png"
    img.save(hi)
    img.resize((820, 312), Image.Resampling.LANCZOS).save(std)
    print(hi)
    print(std)


if __name__ == "__main__":
    render()

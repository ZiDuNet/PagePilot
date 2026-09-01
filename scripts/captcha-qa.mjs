import { inflateSync } from "node:zlib";

const DIGITS = [
  [0b111, 0b101, 0b101, 0b101, 0b111],
  [0b010, 0b110, 0b010, 0b010, 0b111],
  [0b111, 0b001, 0b111, 0b100, 0b111],
  [0b111, 0b001, 0b111, 0b001, 0b111],
  [0b101, 0b101, 0b111, 0b001, 0b001],
  [0b111, 0b100, 0b111, 0b001, 0b111],
  [0b111, 0b100, 0b111, 0b101, 0b111],
  [0b111, 0b001, 0b010, 0b010, 0b010],
  [0b111, 0b101, 0b111, 0b101, 0b111],
  [0b111, 0b101, 0b111, 0b001, 0b111],
];

function paeth(a, b, c) {
  const p = a + b - c;
  const pa = Math.abs(p - a);
  const pb = Math.abs(p - b);
  const pc = Math.abs(p - c);
  return pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
}

function decodePNG(dataURL) {
  const image = String(dataURL?.image || dataURL || "");
  const match = /^data:image\/png;base64,([A-Za-z0-9+/=]+)$/.exec(image);
  if (!match) throw new Error("captcha image is not a PNG data URL");
  const png = Buffer.from(match[1], "base64");
  const signature = Buffer.from("89504e470d0a1a0a", "hex");
  if (png.length < signature.length || !png.subarray(0, signature.length).equals(signature)) {
    throw new Error("captcha PNG signature is invalid");
  }

  let width = 0;
  let height = 0;
  let bitDepth = 0;
  let colorType = 0;
  let interlace = 0;
  const chunks = [];
  let offset = signature.length;
  while (offset + 12 <= png.length) {
    const length = png.readUInt32BE(offset);
    const type = png.toString("ascii", offset + 4, offset + 8);
    const start = offset + 8;
    const end = start + length;
    if (end + 4 > png.length) throw new Error("captcha PNG is truncated");
    const data = png.subarray(start, end);
    if (type === "IHDR") {
      if (length !== 13) throw new Error("captcha PNG header is invalid");
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      bitDepth = data[8];
      colorType = data[9];
      interlace = data[12];
    } else if (type === "IDAT") {
      chunks.push(data);
    } else if (type === "IEND") {
      break;
    }
    offset = end + 4;
  }
  if (!width || !height || bitDepth !== 8 || colorType !== 6 || interlace !== 0 || chunks.length === 0) {
    throw new Error("captcha PNG format is unsupported");
  }

  const bytesPerPixel = 4;
  const stride = width * bytesPerPixel;
  const decoded = inflateSync(Buffer.concat(chunks));
  const expected = height * (stride + 1);
  if (decoded.length < expected) throw new Error("captcha PNG pixels are truncated");
  const pixels = Buffer.alloc(height * stride);
  let sourceOffset = 0;
  for (let y = 0; y < height; y += 1) {
    const filter = decoded[sourceOffset++];
    const rowOffset = y * stride;
    const previousOffset = (y - 1) * stride;
    for (let x = 0; x < stride; x += 1) {
      const raw = decoded[sourceOffset++];
      const left = x >= bytesPerPixel ? pixels[rowOffset + x - bytesPerPixel] : 0;
      const up = y > 0 ? pixels[previousOffset + x] : 0;
      const upperLeft = y > 0 && x >= bytesPerPixel ? pixels[previousOffset + x - bytesPerPixel] : 0;
      let value;
      switch (filter) {
        case 0:
          value = raw;
          break;
        case 1:
          value = raw + left;
          break;
        case 2:
          value = raw + up;
          break;
        case 3:
          value = raw + Math.floor((left + up) / 2);
          break;
        case 4:
          value = raw + paeth(left, up, upperLeft);
          break;
        default:
          throw new Error(`captcha PNG filter ${filter} is unsupported`);
      }
      pixels[rowOffset + x] = value & 0xff;
    }
  }
  return { width, height, pixels };
}

function isDark(pixels, width, height, x, y) {
  if (x < 0 || y < 0 || x >= width || y >= height) return false;
  const offset = (y * width + x) * 4;
  return pixels[offset + 3] >= 220 &&
    pixels[offset] + pixels[offset + 1] + pixels[offset + 2] < 480;
}

function scoreDigit(image, digit, x, y) {
  const pattern = DIGITS[digit];
  let score = 0;
  for (let row = 0; row < 5; row += 1) {
    for (let col = 0; col < 3; col += 1) {
      let dark = 0;
      for (let dy = 0; dy < 6; dy += 1) {
        for (let dx = 0; dx < 6; dx += 1) {
          if (isDark(image.pixels, image.width, image.height, x + col * 6 + dx, y + row * 6 + dy)) dark += 1;
        }
      }
      const filled = dark / 36;
      const expected = (pattern[row] & (1 << (2 - col))) !== 0;
      score += expected ? filled : 1 - filled;
    }
  }
  return score;
}

export function captchaAnswer(captchaOrImage) {
  const image = decodePNG(captchaOrImage);
  let answer = "";
  for (let index = 0; index < 4; index += 1) {
    let bestDigit = 0;
    let bestScore = -Infinity;
    for (let dx = -3; dx <= 3; dx += 1) {
      for (let dy = -3; dy <= 3; dy += 1) {
        const x = 13 + index * 33 + dx;
        const y = 11 + dy;
        for (let digit = 0; digit < DIGITS.length; digit += 1) {
          const score = scoreDigit(image, digit, x, y);
          if (score > bestScore) {
            bestScore = score;
            bestDigit = digit;
          }
        }
      }
    }
    if (bestScore < 12) throw new Error("could not read captcha answer from PNG");
    answer += String(bestDigit);
  }
  return answer;
}

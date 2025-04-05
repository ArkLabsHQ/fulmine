"use strict";
var qr = (() => {
  var __defProp = Object.defineProperty;
  var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
  var __getOwnPropNames = Object.getOwnPropertyNames;
  var __hasOwnProp = Object.prototype.hasOwnProperty;
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };
  var __copyProps = (to, from, except, desc) => {
    if (from && typeof from === "object" || typeof from === "function") {
      for (let key of __getOwnPropNames(from))
        if (!__hasOwnProp.call(to, key) && key !== except)
          __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
    }
    return to;
  };
  var __toCommonJS = (mod2) => __copyProps(__defProp({}, "__esModule", { value: true }), mod2);

  // input.js
  var input_exports = {};
  __export(input_exports, {
    Bitmap: () => Bitmap,
    ECMode: () => ECMode,
    Encoding: () => Encoding,
    decodeQR: () => decodeQR,
    dom: () => dom_exports,
    encodeQR: () => encodeQR
  });

  // ../../esm/index.js
  var chCodes = { newline: 10, reset: 27 };
  function assertNumber(n) {
    if (!Number.isSafeInteger(n))
      throw new Error(`integer expected: ${n}`);
  }
  function validateVersion(ver) {
    if (!Number.isSafeInteger(ver) || ver < 1 || ver > 40)
      throw new Error(`Invalid version=${ver}. Expected number [1..40]`);
  }
  function bin(dec, pad) {
    return dec.toString(2).padStart(pad, "0");
  }
  function mod(a, b) {
    const result = a % b;
    return result >= 0 ? result : b + result;
  }
  function fillArr(length, val) {
    return new Array(length).fill(val);
  }
  function interleaveBytes(...blocks) {
    let len = 0;
    for (const b of blocks)
      len = Math.max(len, b.length);
    const res = [];
    for (let i = 0; i < len; i++) {
      for (const b of blocks) {
        if (i >= b.length)
          continue;
        res.push(b[i]);
      }
    }
    return new Uint8Array(res);
  }
  function includesAt(lst, pattern2, index) {
    if (index < 0 || index + pattern2.length > lst.length)
      return false;
    for (let i = 0; i < pattern2.length; i++)
      if (pattern2[i] !== lst[index + i])
        return false;
    return true;
  }
  function best() {
    let best3;
    let bestScore = Infinity;
    return {
      add(score, value) {
        if (score >= bestScore)
          return;
        best3 = value;
        bestScore = score;
      },
      get: () => best3,
      score: () => bestScore
    };
  }
  function alphabet(alphabet2) {
    return {
      has: (char) => alphabet2.includes(char),
      decode: (input) => {
        if (!Array.isArray(input) || input.length && typeof input[0] !== "string")
          throw new Error("alphabet.decode input should be array of strings");
        return input.map((letter) => {
          if (typeof letter !== "string")
            throw new Error(`alphabet.decode: not string element=${letter}`);
          const index = alphabet2.indexOf(letter);
          if (index === -1)
            throw new Error(`Unknown letter: "${letter}". Allowed: ${alphabet2}`);
          return index;
        });
      },
      encode: (digits) => {
        if (!Array.isArray(digits) || digits.length && typeof digits[0] !== "number")
          throw new Error("alphabet.encode input should be an array of numbers");
        return digits.map((i) => {
          assertNumber(i);
          if (i < 0 || i >= alphabet2.length)
            throw new Error(`Digit index outside alphabet: ${i} (alphabet: ${alphabet2.length})`);
          return alphabet2[i];
        });
      }
    };
  }
  var Bitmap = class _Bitmap {
    static size(size, limit) {
      if (typeof size === "number")
        size = { height: size, width: size };
      if (!Number.isSafeInteger(size.height) && size.height !== Infinity)
        throw new Error(`Bitmap: invalid height=${size.height} (${typeof size.height})`);
      if (!Number.isSafeInteger(size.width) && size.width !== Infinity)
        throw new Error(`Bitmap: invalid width=${size.width} (${typeof size.width})`);
      if (limit !== void 0) {
        size = {
          width: Math.min(size.width, limit.width),
          height: Math.min(size.height, limit.height)
        };
      }
      return size;
    }
    static fromString(s) {
      s = s.replace(/^\n+/g, "").replace(/\n+$/g, "");
      const lines = s.split(String.fromCharCode(chCodes.newline));
      const height = lines.length;
      const data = new Array(height);
      let width;
      for (const line of lines) {
        const row = line.split("").map((i) => {
          if (i === "X")
            return true;
          if (i === " ")
            return false;
          if (i === "?")
            return void 0;
          throw new Error(`Bitmap.fromString: unknown symbol=${i}`);
        });
        if (width && row.length !== width)
          throw new Error(`Bitmap.fromString different row sizes: width=${width} cur=${row.length}`);
        width = row.length;
        data.push(row);
      }
      if (!width)
        width = 0;
      return new _Bitmap({ height, width }, data);
    }
    constructor(size, data) {
      const { height, width } = _Bitmap.size(size);
      this.data = data || Array.from({ length: height }, () => fillArr(width, void 0));
      this.height = height;
      this.width = width;
    }
    point(p) {
      return this.data[p.y][p.x];
    }
    isInside(p) {
      return 0 <= p.x && p.x < this.width && 0 <= p.y && p.y < this.height;
    }
    size(offset) {
      if (!offset)
        return { height: this.height, width: this.width };
      const { x, y } = this.xy(offset);
      return { height: this.height - y, width: this.width - x };
    }
    xy(c) {
      if (typeof c === "number")
        c = { x: c, y: c };
      if (!Number.isSafeInteger(c.x))
        throw new Error(`Bitmap: invalid x=${c.x}`);
      if (!Number.isSafeInteger(c.y))
        throw new Error(`Bitmap: invalid y=${c.y}`);
      c.x = mod(c.x, this.width);
      c.y = mod(c.y, this.height);
      return c;
    }
    // Basically every operation can be represented as rect
    rect(c, size, value) {
      const { x, y } = this.xy(c);
      const { height, width } = _Bitmap.size(size, this.size({ x, y }));
      for (let yPos = 0; yPos < height; yPos++) {
        for (let xPos = 0; xPos < width; xPos++) {
          this.data[y + yPos][x + xPos] = typeof value === "function" ? value({ x: xPos, y: yPos }, this.data[y + yPos][x + xPos]) : value;
        }
      }
      return this;
    }
    // returns rectangular part of bitmap
    rectRead(c, size, fn) {
      return this.rect(c, size, (c2, cur) => {
        fn(c2, cur);
        return cur;
      });
    }
    // Horizontal & vertical lines
    hLine(c, len, value) {
      return this.rect(c, { width: len, height: 1 }, value);
    }
    vLine(c, len, value) {
      return this.rect(c, { width: 1, height: len }, value);
    }
    // add border
    border(border = 2, value) {
      const height = this.height + 2 * border;
      const width = this.width + 2 * border;
      const v = fillArr(border, value);
      const h = Array.from({ length: border }, () => fillArr(width, value));
      return new _Bitmap({ height, width }, [...h, ...this.data.map((i) => [...v, ...i, ...v]), ...h]);
    }
    // Embed another bitmap on coordinates
    embed(c, bm) {
      return this.rect(c, bm.size(), ({ x, y }) => bm.data[y][x]);
    }
    // returns rectangular part of bitmap
    rectSlice(c, size = this.size()) {
      const rect = new _Bitmap(_Bitmap.size(size, this.size(this.xy(c))));
      this.rect(c, size, ({ x, y }, cur) => rect.data[y][x] = cur);
      return rect;
    }
    // Change shape, replace rows with columns (data[y][x] -> data[x][y])
    inverse() {
      const { height, width } = this;
      const res = new _Bitmap({ height: width, width: height });
      return res.rect({ x: 0, y: 0 }, Infinity, ({ x, y }) => this.data[x][y]);
    }
    // Each pixel size is multiplied by factor
    scale(factor) {
      if (!Number.isSafeInteger(factor) || factor > 1024)
        throw new Error(`invalid scale factor: ${factor}`);
      const { height, width } = this;
      const res = new _Bitmap({ height: factor * height, width: factor * width });
      return res.rect({ x: 0, y: 0 }, Infinity, ({ x, y }) => this.data[Math.floor(y / factor)][Math.floor(x / factor)]);
    }
    clone() {
      const res = new _Bitmap(this.size());
      return res.rect({ x: 0, y: 0 }, this.size(), ({ x, y }) => this.data[y][x]);
    }
    // Ensure that there is no undefined values left
    assertDrawn() {
      this.rectRead(0, Infinity, (_, cur) => {
        if (typeof cur !== "boolean")
          throw new Error(`Invalid color type=${typeof cur}`);
      });
    }
    // Simple string representation for debugging
    toString() {
      return this.data.map((i) => i.map((j) => j === void 0 ? "?" : j ? "X" : " ").join("")).join(String.fromCharCode(chCodes.newline));
    }
    toASCII() {
      const { height, width, data } = this;
      let out = "";
      for (let y = 0; y < height; y += 2) {
        for (let x = 0; x < width; x++) {
          const first = data[y][x];
          const second = y + 1 >= height ? true : data[y + 1][x];
          if (!first && !second)
            out += "\u2588";
          else if (!first && second)
            out += "\u2580";
          else if (first && !second)
            out += "\u2584";
          else if (first && second)
            out += " ";
        }
        out += String.fromCharCode(chCodes.newline);
      }
      return out;
    }
    toTerm() {
      const cc = String.fromCharCode(chCodes.reset);
      const reset = cc + "[0m";
      const whiteBG = cc + "[1;47m  " + reset;
      const darkBG = cc + `[40m  ` + reset;
      return this.data.map((i) => i.map((j) => j ? darkBG : whiteBG).join("")).join(String.fromCharCode(chCodes.newline));
    }
    toSVG() {
      let out = `<svg xmlns:svg="http://www.w3.org/2000/svg" viewBox="0 0 ${this.width} ${this.height}" version="1.1" xmlns="http://www.w3.org/2000/svg">`;
      this.rectRead(0, Infinity, ({ x, y }, val) => {
        if (val)
          out += `<rect x="${x}" y="${y}" width="1" height="1" />`;
      });
      out += "</svg>";
      return out;
    }
    toGIF() {
      const u16le = (i) => [i & 255, i >>> 8 & 255];
      const dims = [...u16le(this.width), ...u16le(this.height)];
      const data = [];
      this.rectRead(0, Infinity, (_, cur) => data.push(+(cur === true)));
      const N = 126;
      const bytes = [
        71,
        73,
        70,
        56,
        55,
        97,
        ...dims,
        246,
        0,
        0,
        255,
        255,
        255,
        ...fillArr(3 * 127, 0),
        44,
        0,
        0,
        0,
        0,
        ...dims,
        0,
        7
      ];
      const fullChunks = Math.floor(data.length / N);
      for (let i = 0; i < fullChunks; i++)
        bytes.push(N + 1, 128, ...data.slice(N * i, N * (i + 1)).map((i2) => +i2));
      bytes.push(data.length % N + 1, 128, ...data.slice(fullChunks * N).map((i) => +i));
      bytes.push(1, 129, 0, 59);
      return new Uint8Array(bytes);
    }
    toImage(isRGB = false) {
      const { height, width } = this.size();
      const data = new Uint8Array(height * width * (isRGB ? 3 : 4));
      let i = 0;
      for (let y = 0; y < height; y++) {
        for (let x = 0; x < width; x++) {
          const value = !!this.data[y][x] ? 0 : 255;
          data[i++] = value;
          data[i++] = value;
          data[i++] = value;
          if (!isRGB)
            data[i++] = 255;
        }
      }
      return { height, width, data };
    }
  };
  var ECMode = ["low", "medium", "quartile", "high"];
  var Encoding = ["numeric", "alphanumeric", "byte", "kanji", "eci"];
  var BYTES = [
    // 1,  2,  3,   4,   5,   6,   7,   8,   9,  10,  11,  12,  13,  14,  15,  16,  17,  18,  19,   20,
    26,
    44,
    70,
    100,
    134,
    172,
    196,
    242,
    292,
    346,
    404,
    466,
    532,
    581,
    655,
    733,
    815,
    901,
    991,
    1085,
    //  21,   22,   23,   24,   25,   26,   27,   28,   29,   30,   31,   32,   33,   34,   35,   36,   37,   38,   39,   40
    1156,
    1258,
    1364,
    1474,
    1588,
    1706,
    1828,
    1921,
    2051,
    2185,
    2323,
    2465,
    2611,
    2761,
    2876,
    3034,
    3196,
    3362,
    3532,
    3706
  ];
  var WORDS_PER_BLOCK = {
    // Version 1,  2,  3,  4,  5,  6,  7,  8,  9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40
    low: [7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30],
    medium: [10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28],
    quartile: [13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30],
    high: [17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30]
  };
  var ECC_BLOCKS = {
    // Version   1, 2, 3, 4, 5, 6, 7, 8, 9,10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40
    low: [1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25],
    medium: [1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49],
    quartile: [1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68],
    high: [1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81]
  };
  var info = {
    size: {
      encode: (ver) => 21 + 4 * (ver - 1),
      // ver1 = 21, ver40=177 blocks
      decode: (size) => (size - 17) / 4
    },
    sizeType: (ver) => Math.floor((ver + 7) / 17),
    // Based on https://codereview.stackexchange.com/questions/74925/algorithm-to-generate-this-alignment-pattern-locations-table-for-qr-codes
    alignmentPatterns(ver) {
      if (ver === 1)
        return [];
      const first = 6;
      const last = info.size.encode(ver) - first - 1;
      const distance3 = last - first;
      const count = Math.ceil(distance3 / 28);
      let interval = Math.floor(distance3 / count);
      if (interval % 2)
        interval += 1;
      else if (distance3 % count * 2 >= count)
        interval += 2;
      const res = [first];
      for (let m = 1; m < count; m++)
        res.push(last - (count - m) * interval);
      res.push(last);
      return res;
    },
    ECCode: {
      low: 1,
      medium: 0,
      quartile: 3,
      high: 2
    },
    formatMask: 21522,
    formatBits(ecc, maskIdx) {
      const data = info.ECCode[ecc] << 3 | maskIdx;
      let d = data;
      for (let i = 0; i < 10; i++)
        d = d << 1 ^ (d >> 9) * 1335;
      return (data << 10 | d) ^ info.formatMask;
    },
    versionBits(ver) {
      let d = ver;
      for (let i = 0; i < 12; i++)
        d = d << 1 ^ (d >> 11) * 7973;
      return ver << 12 | d;
    },
    alphabet: {
      numeric: alphabet("0123456789"),
      alphanumerc: alphabet("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:")
    },
    // as Record<EncodingType, ReturnType<typeof alphabet>>,
    lengthBits(ver, type) {
      const table = {
        numeric: [10, 12, 14],
        alphanumeric: [9, 11, 13],
        byte: [8, 16, 16],
        kanji: [8, 10, 12],
        eci: [0, 0, 0]
      };
      return table[type][info.sizeType(ver)];
    },
    modeBits: {
      numeric: "0001",
      alphanumeric: "0010",
      byte: "0100",
      kanji: "1000",
      eci: "0111"
    },
    capacity(ver, ecc) {
      const bytes = BYTES[ver - 1];
      const words = WORDS_PER_BLOCK[ecc][ver - 1];
      const numBlocks = ECC_BLOCKS[ecc][ver - 1];
      const blockLen = Math.floor(bytes / numBlocks) - words;
      const shortBlocks = numBlocks - bytes % numBlocks;
      return {
        words,
        numBlocks,
        shortBlocks,
        blockLen,
        capacity: (bytes - words * numBlocks) * 8,
        total: (words + blockLen) * numBlocks + numBlocks - shortBlocks
      };
    }
  };
  var PATTERNS = [
    (x, y) => (x + y) % 2 == 0,
    (_x, y) => y % 2 == 0,
    (x, _y) => x % 3 == 0,
    (x, y) => (x + y) % 3 == 0,
    (x, y) => (Math.floor(y / 2) + Math.floor(x / 3)) % 2 == 0,
    (x, y) => x * y % 2 + x * y % 3 == 0,
    (x, y) => (x * y % 2 + x * y % 3) % 2 == 0,
    (x, y) => ((x + y) % 2 + x * y % 3) % 2 == 0
  ];
  var GF = {
    tables: ((p_poly) => {
      const exp = fillArr(256, 0);
      const log = fillArr(256, 0);
      for (let i = 0, x = 1; i < 256; i++) {
        exp[i] = x;
        log[x] = i;
        x <<= 1;
        if (x & 256)
          x ^= p_poly;
      }
      return { exp, log };
    })(285),
    exp: (x) => GF.tables.exp[x],
    log(x) {
      if (x === 0)
        throw new Error(`GF.log: invalid arg=${x}`);
      return GF.tables.log[x] % 255;
    },
    mul(x, y) {
      if (x === 0 || y === 0)
        return 0;
      return GF.tables.exp[(GF.tables.log[x] + GF.tables.log[y]) % 255];
    },
    add: (x, y) => x ^ y,
    pow: (x, e) => GF.tables.exp[GF.tables.log[x] * e % 255],
    inv(x) {
      if (x === 0)
        throw new Error(`GF.inverse: invalid arg=${x}`);
      return GF.tables.exp[255 - GF.tables.log[x]];
    },
    polynomial(poly) {
      if (poly.length == 0)
        throw new Error("GF.polymomial: invalid length");
      if (poly[0] !== 0)
        return poly;
      let i = 0;
      for (; i < poly.length - 1 && poly[i] == 0; i++)
        ;
      return poly.slice(i);
    },
    monomial(degree, coefficient) {
      if (degree < 0)
        throw new Error(`GF.monomial: invalid degree=${degree}`);
      if (coefficient == 0)
        return [0];
      let coefficients = fillArr(degree + 1, 0);
      coefficients[0] = coefficient;
      return GF.polynomial(coefficients);
    },
    degree: (a) => a.length - 1,
    coefficient: (a, degree) => a[GF.degree(a) - degree],
    mulPoly(a, b) {
      if (a[0] === 0 || b[0] === 0)
        return [0];
      const res = fillArr(a.length + b.length - 1, 0);
      for (let i = 0; i < a.length; i++) {
        for (let j = 0; j < b.length; j++) {
          res[i + j] = GF.add(res[i + j], GF.mul(a[i], b[j]));
        }
      }
      return GF.polynomial(res);
    },
    mulPolyScalar(a, scalar) {
      if (scalar == 0)
        return [0];
      if (scalar == 1)
        return a;
      const res = fillArr(a.length, 0);
      for (let i = 0; i < a.length; i++)
        res[i] = GF.mul(a[i], scalar);
      return GF.polynomial(res);
    },
    mulPolyMonomial(a, degree, coefficient) {
      if (degree < 0)
        throw new Error("GF.mulPolyMonomial: invalid degree");
      if (coefficient == 0)
        return [0];
      const res = fillArr(a.length + degree, 0);
      for (let i = 0; i < a.length; i++)
        res[i] = GF.mul(a[i], coefficient);
      return GF.polynomial(res);
    },
    addPoly(a, b) {
      if (a[0] === 0)
        return b;
      if (b[0] === 0)
        return a;
      let smaller = a;
      let larger = b;
      if (smaller.length > larger.length)
        [smaller, larger] = [larger, smaller];
      let sumDiff = fillArr(larger.length, 0);
      let lengthDiff = larger.length - smaller.length;
      let s = larger.slice(0, lengthDiff);
      for (let i = 0; i < s.length; i++)
        sumDiff[i] = s[i];
      for (let i = lengthDiff; i < larger.length; i++)
        sumDiff[i] = GF.add(smaller[i - lengthDiff], larger[i]);
      return GF.polynomial(sumDiff);
    },
    remainderPoly(data, divisor) {
      const out = Array.from(data);
      for (let i = 0; i < data.length - divisor.length + 1; i++) {
        const elm = out[i];
        if (elm === 0)
          continue;
        for (let j = 1; j < divisor.length; j++) {
          if (divisor[j] !== 0)
            out[i + j] = GF.add(out[i + j], GF.mul(divisor[j], elm));
        }
      }
      return out.slice(data.length - divisor.length + 1, out.length);
    },
    divisorPoly(degree) {
      let g = [1];
      for (let i = 0; i < degree; i++)
        g = GF.mulPoly(g, [1, GF.pow(2, i)]);
      return g;
    },
    evalPoly(poly, a) {
      if (a == 0)
        return GF.coefficient(poly, 0);
      let res = poly[0];
      for (let i = 1; i < poly.length; i++)
        res = GF.add(GF.mul(a, res), poly[i]);
      return res;
    },
    // TODO: cleanup
    euclidian(a, b, R) {
      if (GF.degree(a) < GF.degree(b))
        [a, b] = [b, a];
      let rLast = a;
      let r = b;
      let tLast = [0];
      let t = [1];
      while (2 * GF.degree(r) >= R) {
        let rLastLast = rLast;
        let tLastLast = tLast;
        rLast = r;
        tLast = t;
        if (rLast[0] === 0)
          throw new Error("rLast[0] === 0");
        r = rLastLast;
        let q = [0];
        const dltInverse = GF.inv(rLast[0]);
        while (GF.degree(r) >= GF.degree(rLast) && r[0] !== 0) {
          const degreeDiff = GF.degree(r) - GF.degree(rLast);
          const scale = GF.mul(r[0], dltInverse);
          q = GF.addPoly(q, GF.monomial(degreeDiff, scale));
          r = GF.addPoly(r, GF.mulPolyMonomial(rLast, degreeDiff, scale));
        }
        q = GF.mulPoly(q, tLast);
        t = GF.addPoly(q, tLastLast);
        if (GF.degree(r) >= GF.degree(rLast))
          throw new Error(`Division failed r: ${r}, rLast: ${rLast}`);
      }
      const sigmaTildeAtZero = GF.coefficient(t, 0);
      if (sigmaTildeAtZero == 0)
        throw new Error("sigmaTilde(0) was zero");
      const inverse = GF.inv(sigmaTildeAtZero);
      return [GF.mulPolyScalar(t, inverse), GF.mulPolyScalar(r, inverse)];
    }
  };
  function RS(eccWords) {
    return {
      encode(from) {
        const d = GF.divisorPoly(eccWords);
        const pol = Array.from(from);
        pol.push(...d.slice(0, -1).fill(0));
        return Uint8Array.from(GF.remainderPoly(pol, d));
      },
      decode(to) {
        const res = to.slice();
        const poly = GF.polynomial(Array.from(to));
        let syndrome = fillArr(eccWords, 0);
        let hasError = false;
        for (let i = 0; i < eccWords; i++) {
          const evl = GF.evalPoly(poly, GF.exp(i));
          syndrome[syndrome.length - 1 - i] = evl;
          if (evl !== 0)
            hasError = true;
        }
        if (!hasError)
          return res;
        syndrome = GF.polynomial(syndrome);
        const monomial = GF.monomial(eccWords, 1);
        const [errorLocator, errorEvaluator] = GF.euclidian(monomial, syndrome, eccWords);
        const locations = fillArr(GF.degree(errorLocator), 0);
        let e = 0;
        for (let i = 1; i < 256 && e < locations.length; i++) {
          if (GF.evalPoly(errorLocator, i) === 0)
            locations[e++] = GF.inv(i);
        }
        if (e !== locations.length)
          throw new Error("RS.decode: invalid errors number");
        for (let i = 0; i < locations.length; i++) {
          const pos = res.length - 1 - GF.log(locations[i]);
          if (pos < 0)
            throw new Error("RS.decode: invalid error location");
          const xiInverse = GF.inv(locations[i]);
          let denominator = 1;
          for (let j = 0; j < locations.length; j++) {
            if (i === j)
              continue;
            denominator = GF.mul(denominator, GF.add(1, GF.mul(locations[j], xiInverse)));
          }
          res[pos] = GF.add(res[pos], GF.mul(GF.evalPoly(errorEvaluator, xiInverse), GF.inv(denominator)));
        }
        return res;
      }
    };
  }
  function interleave(ver, ecc) {
    const { words, shortBlocks, numBlocks, blockLen, total } = info.capacity(ver, ecc);
    const rs = RS(words);
    return {
      encode(bytes) {
        const blocks = [];
        const eccBlocks = [];
        for (let i = 0; i < numBlocks; i++) {
          const isShort = i < shortBlocks;
          const len = blockLen + (isShort ? 0 : 1);
          blocks.push(bytes.subarray(0, len));
          eccBlocks.push(rs.encode(bytes.subarray(0, len)));
          bytes = bytes.subarray(len);
        }
        const resBlocks = interleaveBytes(...blocks);
        const resECC = interleaveBytes(...eccBlocks);
        const res = new Uint8Array(resBlocks.length + resECC.length);
        res.set(resBlocks);
        res.set(resECC, resBlocks.length);
        return res;
      },
      decode(data) {
        if (data.length !== total)
          throw new Error(`interleave.decode: len(data)=${data.length}, total=${total}`);
        const blocks = [];
        for (let i = 0; i < numBlocks; i++) {
          const isShort = i < shortBlocks;
          blocks.push(new Uint8Array(words + blockLen + (isShort ? 0 : 1)));
        }
        let pos = 0;
        for (let i = 0; i < blockLen; i++) {
          for (let j = 0; j < numBlocks; j++)
            blocks[j][i] = data[pos++];
        }
        for (let j = shortBlocks; j < numBlocks; j++)
          blocks[j][blockLen] = data[pos++];
        for (let i = blockLen; i < blockLen + words; i++) {
          for (let j = 0; j < numBlocks; j++) {
            const isShort = j < shortBlocks;
            blocks[j][i + (isShort ? 0 : 1)] = data[pos++];
          }
        }
        const res = [];
        for (const block of blocks)
          res.push(...Array.from(rs.decode(block)).slice(0, -words));
        return Uint8Array.from(res);
      }
    };
  }
  function drawTemplate(ver, ecc, maskIdx, test = false) {
    const size = info.size.encode(ver);
    let b = new Bitmap(size + 2);
    const finder = new Bitmap(3).rect(0, 3, true).border(1, false).border(1, true).border(1, false);
    b = b.embed(0, finder).embed({ x: -finder.width, y: 0 }, finder).embed({ x: 0, y: -finder.height }, finder);
    b = b.rectSlice(1, size);
    const align = new Bitmap(1).rect(0, 1, true).border(1, false).border(1, true);
    const alignPos = info.alignmentPatterns(ver);
    for (const y of alignPos) {
      for (const x of alignPos) {
        if (b.data[y][x] !== void 0)
          continue;
        b.embed({ x: x - 2, y: y - 2 }, align);
      }
    }
    b = b.hLine({ x: 0, y: 6 }, Infinity, ({ x }, cur) => cur === void 0 ? x % 2 == 0 : cur).vLine({ x: 6, y: 0 }, Infinity, ({ y }, cur) => cur === void 0 ? y % 2 == 0 : cur);
    {
      const bits = info.formatBits(ecc, maskIdx);
      const getBit = (i) => !test && (bits >> i & 1) == 1;
      for (let i = 0; i < 6; i++)
        b.data[i][8] = getBit(i);
      for (let i = 6; i < 8; i++)
        b.data[i + 1][8] = getBit(i);
      for (let i = 8; i < 15; i++)
        b.data[size - 15 + i][8] = getBit(i);
      for (let i = 0; i < 8; i++)
        b.data[8][size - i - 1] = getBit(i);
      for (let i = 8; i < 9; i++)
        b.data[8][15 - i - 1 + 1] = getBit(i);
      for (let i = 9; i < 15; i++)
        b.data[8][15 - i - 1] = getBit(i);
      b.data[size - 8][8] = !test;
    }
    if (ver >= 7) {
      const bits = info.versionBits(ver);
      for (let i = 0; i < 18; i += 1) {
        const bit = !test && (bits >> i & 1) == 1;
        const x = Math.floor(i / 3);
        const y = i % 3 + size - 8 - 3;
        b.data[x][y] = bit;
        b.data[y][x] = bit;
      }
    }
    return b;
  }
  function zigzag(tpl, maskIdx, fn) {
    const size = tpl.height;
    const pattern2 = PATTERNS[maskIdx];
    let dir = -1;
    let y = size - 1;
    for (let xOffset = size - 1; xOffset > 0; xOffset -= 2) {
      if (xOffset == 6)
        xOffset = 5;
      for (; ; y += dir) {
        for (let j = 0; j < 2; j += 1) {
          const x = xOffset - j;
          if (tpl.data[y][x] !== void 0)
            continue;
          fn(x, y, pattern2(x, y));
        }
        if (y + dir < 0 || y + dir >= size)
          break;
      }
      dir = -dir;
    }
  }
  function detectType(str) {
    let type = "numeric";
    for (let x of str) {
      if (info.alphabet.numeric.has(x))
        continue;
      type = "alphanumeric";
      if (!info.alphabet.alphanumerc.has(x))
        return "byte";
    }
    return type;
  }
  function utf8ToBytes(str) {
    if (typeof str !== "string")
      throw new Error(`utf8ToBytes expected string, got ${typeof str}`);
    return new Uint8Array(new TextEncoder().encode(str));
  }
  function encode(ver, ecc, data, type) {
    let encoded = "";
    let dataLen = data.length;
    if (type === "numeric") {
      const t = info.alphabet.numeric.decode(data.split(""));
      const n = t.length;
      for (let i = 0; i < n - 2; i += 3)
        encoded += bin(t[i] * 100 + t[i + 1] * 10 + t[i + 2], 10);
      if (n % 3 === 1) {
        encoded += bin(t[n - 1], 4);
      } else if (n % 3 === 2) {
        encoded += bin(t[n - 2] * 10 + t[n - 1], 7);
      }
    } else if (type === "alphanumeric") {
      const t = info.alphabet.alphanumerc.decode(data.split(""));
      const n = t.length;
      for (let i = 0; i < n - 1; i += 2)
        encoded += bin(t[i] * 45 + t[i + 1], 11);
      if (n % 2 == 1)
        encoded += bin(t[n - 1], 6);
    } else if (type === "byte") {
      const utf8 = utf8ToBytes(data);
      dataLen = utf8.length;
      encoded = Array.from(utf8).map((i) => bin(i, 8)).join("");
    } else {
      throw new Error("encode: unsupported type");
    }
    const { capacity } = info.capacity(ver, ecc);
    const len = bin(dataLen, info.lengthBits(ver, type));
    let bits = info.modeBits[type] + len + encoded;
    if (bits.length > capacity)
      throw new Error("Capacity overflow");
    bits += "0".repeat(Math.min(4, Math.max(0, capacity - bits.length)));
    if (bits.length % 8)
      bits += "0".repeat(8 - bits.length % 8);
    const padding = "1110110000010001";
    for (let idx = 0; bits.length !== capacity; idx++)
      bits += padding[idx % padding.length];
    const bytes = Uint8Array.from(bits.match(/(.{8})/g).map((i) => Number(`0b${i}`)));
    return interleave(ver, ecc).encode(bytes);
  }
  function drawQR(ver, ecc, data, maskIdx, test = false) {
    const b = drawTemplate(ver, ecc, maskIdx, test);
    let i = 0;
    const need = 8 * data.length;
    zigzag(b, maskIdx, (x, y, mask) => {
      let value = false;
      if (i < need) {
        value = (data[i >>> 3] >> (7 - i & 7) & 1) !== 0;
        i++;
      }
      b.data[y][x] = value !== mask;
    });
    if (i !== need)
      throw new Error("QR: bytes left after draw");
    return b;
  }
  function penalty(bm) {
    const inverse = bm.inverse();
    const sameColor = (row) => {
      let res = 0;
      for (let i = 0, same = 1, last = void 0; i < row.length; i++) {
        if (last === row[i]) {
          same++;
          if (i !== row.length - 1)
            continue;
        }
        if (same >= 5)
          res += 3 + (same - 5);
        last = row[i];
        same = 1;
      }
      return res;
    };
    let adjacent = 0;
    bm.data.forEach((row) => adjacent += sameColor(row));
    inverse.data.forEach((column) => adjacent += sameColor(column));
    let box = 0;
    let b = bm.data;
    const lastW = bm.width - 1;
    const lastH = bm.height - 1;
    for (let x = 0; x < lastW; x++) {
      for (let y = 0; y < lastH; y++) {
        const x1 = x + 1;
        const y1 = y + 1;
        if (b[x][y] === b[x1][y] && b[x1][y] === b[x][y1] && b[x1][y] === b[x1][y1]) {
          box += 3;
        }
      }
    }
    const finderPattern = (row) => {
      const finderPattern2 = [true, false, true, true, true, false, true];
      const lightPattern = [false, false, false, false];
      const p1 = [...finderPattern2, ...lightPattern];
      const p2 = [...lightPattern, ...finderPattern2];
      let res = 0;
      for (let i = 0; i < row.length; i++) {
        if (includesAt(row, p1, i))
          res += 40;
        if (includesAt(row, p2, i))
          res += 40;
      }
      return res;
    };
    let finder = 0;
    for (const row of bm.data)
      finder += finderPattern(row);
    for (const column of inverse.data)
      finder += finderPattern(column);
    let darkPixels = 0;
    bm.rectRead(0, Infinity, (_c, val) => darkPixels += val ? 1 : 0);
    const darkPercent = darkPixels / (bm.height * bm.width) * 100;
    const dark = 10 * Math.floor(Math.abs(darkPercent - 50) / 5);
    return adjacent + box + finder + dark;
  }
  function drawQRBest(ver, ecc, data, maskIdx) {
    if (maskIdx === void 0) {
      const bestMask = best();
      for (let mask = 0; mask < PATTERNS.length; mask++)
        bestMask.add(penalty(drawQR(ver, ecc, data, mask, true)), mask);
      maskIdx = bestMask.get();
    }
    if (maskIdx === void 0)
      throw new Error("Cannot find mask");
    return drawQR(ver, ecc, data, maskIdx);
  }
  function validateECC(ec) {
    if (!ECMode.includes(ec))
      throw new Error(`Invalid error correction mode=${ec}. Expected: ${ECMode}`);
  }
  function validateEncoding(enc) {
    if (!Encoding.includes(enc))
      throw new Error(`Encoding: invalid mode=${enc}. Expected: ${Encoding}`);
    if (enc === "kanji" || enc === "eci")
      throw new Error(`Encoding: ${enc} is not supported (yet?).`);
  }
  function validateMask(mask) {
    if (![0, 1, 2, 3, 4, 5, 6, 7].includes(mask) || !PATTERNS[mask])
      throw new Error(`Invalid mask=${mask}. Expected number [0..7]`);
  }
  function encodeQR(text, output = "raw", opts = {}) {
    const ecc = opts.ecc !== void 0 ? opts.ecc : "medium";
    validateECC(ecc);
    const encoding = opts.encoding !== void 0 ? opts.encoding : detectType(text);
    validateEncoding(encoding);
    if (opts.mask !== void 0)
      validateMask(opts.mask);
    let ver = opts.version;
    let data, err = new Error("Unknown error");
    if (ver !== void 0) {
      validateVersion(ver);
      data = encode(ver, ecc, text, encoding);
    } else {
      for (let i = 1; i <= 40; i++) {
        try {
          data = encode(i, ecc, text, encoding);
          ver = i;
          break;
        } catch (e) {
          err = e;
        }
      }
    }
    if (!ver || !data)
      throw err;
    let res = drawQRBest(ver, ecc, data, opts.mask);
    res.assertDrawn();
    const border = opts.border === void 0 ? 2 : opts.border;
    if (!Number.isSafeInteger(border))
      throw new Error(`invalid border type=${typeof border}`);
    res = res.border(border, false);
    if (opts.scale !== void 0)
      res = res.scale(opts.scale);
    if (output === "raw")
      return res.data;
    else if (output === "ascii")
      return res.toASCII();
    else if (output === "svg")
      return res.toSVG();
    else if (output === "gif")
      return res.toGIF();
    else if (output === "term")
      return res.toTerm();
    else
      throw new Error(`Unknown output: ${output}`);
  }
  var utils = {
    best,
    bin,
    drawTemplate,
    fillArr,
    info,
    interleave,
    validateVersion,
    zigzag
  };

  // ../../esm/decode.js
  var { best: best2, bin: bin2, drawTemplate: drawTemplate2, fillArr: fillArr2, info: info2, interleave: interleave2, validateVersion: validateVersion2, zigzag: zigzag2 } = utils;
  var MAX_BITS_ERROR = 3;
  var GRAYSCALE_BLOCK_SIZE = 8;
  var GRAYSCALE_RANGE = 24;
  var PATTERN_VARIANCE = 2;
  var PATTERN_VARIANCE_DIAGONAL = 1.333;
  var PATTERN_MIN_CONFIRMATIONS = 2;
  var DETECT_MIN_ROW_SKIP = 3;
  var int = (n) => n >>> 0;
  var distance2 = (p1, p2) => {
    const x = p1.x - p2.x;
    const y = p1.y - p2.y;
    return x * x + y * y;
  };
  var distance = (p1, p2) => Math.sqrt(distance2(p1, p2));
  var sum = (lst) => lst.reduce((acc, i) => acc + i);
  var pointIncr = (p, incr) => {
    p.x += incr.x;
    p.y += incr.y;
  };
  var pointNeg = (p) => ({ x: -p.x, y: -p.y });
  var pointMirror = (p) => ({ x: p.y, y: p.x });
  var pointClone = (p) => ({ x: p.x, y: p.y });
  var pointInt = (p) => ({ x: int(p.x), y: int(p.y) });
  var pointAdd = (a, b) => ({ x: a.x + b.x, y: a.y + b.y });
  function cap(value, min, max) {
    return Math.max(Math.min(value, max || value), min || value);
  }
  var getBytesPerPixel = (img) => {
    const perPixel = img.data.length / (img.width * img.height);
    if (perPixel === 3 || perPixel === 4)
      return perPixel;
    throw new Error(`Unknown image format, bytes per pixel=${perPixel}`);
  };
  function toBitmap(img) {
    const bytesPerPixel = getBytesPerPixel(img);
    const brightness = new Uint8Array(img.height * img.width);
    for (let i = 0, j = 0, d = img.data; i < d.length; i += bytesPerPixel) {
      const r = d[i];
      const g = d[i + 1];
      const b = d[i + 2];
      brightness[j++] = int((r + 2 * g + b) / 4) & 255;
    }
    const block = GRAYSCALE_BLOCK_SIZE;
    if (img.width < block * 5 || img.height < block * 5)
      throw new Error("image too small");
    const bWidth = Math.ceil(img.width / block);
    const bHeight = Math.ceil(img.height / block);
    const maxY = img.height - block;
    const maxX = img.width - block;
    const blocks = new Uint8Array(bWidth * bHeight);
    for (let y = 0; y < bHeight; y++) {
      const yPos = cap(y * block, 0, maxY);
      for (let x = 0; x < bWidth; x++) {
        const xPos = cap(x * block, 0, maxX);
        let sum2 = 0;
        let min = 255;
        let max = 0;
        for (let yy = 0, pos = yPos * img.width + xPos; yy < block; yy = yy + 1, pos = pos + img.width) {
          for (let xx = 0; xx < block; xx++) {
            const pixel = brightness[pos + xx];
            sum2 += pixel;
            min = Math.min(min, pixel);
            max = Math.max(max, pixel);
          }
        }
        let average = Math.floor(sum2 / block ** 2);
        if (max - min <= GRAYSCALE_RANGE) {
          average = min / 2;
          if (y > 0 && x > 0) {
            const idx = (x2, y2) => y2 * bWidth + x2;
            const prev = (blocks[idx(x, y - 1)] + 2 * blocks[idx(x - 1, y)] + blocks[idx(x - 1, y - 1)]) / 4;
            if (min < prev)
              average = prev;
          }
        }
        blocks[bWidth * y + x] = int(average);
      }
    }
    const matrix = new Bitmap({ width: img.width, height: img.height });
    for (let y = 0; y < bHeight; y++) {
      const yPos = cap(y * block, 0, maxY);
      const top = cap(y, 2, bHeight - 3);
      for (let x = 0; x < bWidth; x++) {
        const xPos = cap(x * block, 0, maxX);
        const left = cap(x, 2, bWidth - 3);
        let sum2 = 0;
        for (let yy = -2; yy <= 2; yy++) {
          const y2 = bWidth * (top + yy) + left;
          for (let xx = -2; xx <= 2; xx++)
            sum2 += blocks[y2 + xx];
        }
        const average = sum2 / 25;
        for (let y2 = 0, pos = yPos * img.width + xPos; y2 < block; y2 += 1, pos += img.width) {
          for (let x2 = 0; x2 < block; x2++) {
            if (brightness[pos + x2] <= average)
              matrix.data[yPos + y2][xPos + x2] = true;
          }
        }
      }
    }
    return matrix;
  }
  function patternEquals(p, p2) {
    if (Math.abs(p2.y - p.y) <= p2.moduleSize && Math.abs(p2.x - p.x) <= p2.moduleSize) {
      const diff = Math.abs(p2.moduleSize - p.moduleSize);
      return diff <= 1 || diff <= p.moduleSize;
    }
    return false;
  }
  function patternMerge(a, b) {
    const count = a.count + b.count;
    return {
      x: (a.count * a.x + b.count * b.x) / count,
      y: (a.count * a.y + b.count * b.y) / count,
      moduleSize: (a.count * a.moduleSize + b.count * b.moduleSize) / count,
      count
    };
  }
  var patternsConfirmed = (lst) => lst.filter((i) => i.count >= PATTERN_MIN_CONFIRMATIONS);
  function pattern(p, size) {
    const _size = size || fillArr2(p.length, 1);
    if (p.length !== _size.length)
      throw new Error("invalid pattern");
    if (!(p.length & 1))
      throw new Error("invalid pattern, length should be odd");
    const res = {
      center: Math.ceil(p.length / 2) - 1,
      length: p.length,
      pattern: p,
      size: _size,
      runs: () => fillArr2(p.length, 0),
      totalSize: sum(_size),
      total: (runs) => runs.reduce((acc, i) => acc + i),
      shift: (runs, n) => {
        for (let i = 0; i < runs.length - n; i++)
          runs[i] = runs[i + 2];
        for (let i = runs.length - n; i < runs.length; i++)
          runs[i] = 0;
      },
      checkSize(runs, moduleSize, v = PATTERN_VARIANCE) {
        const variance = moduleSize / v;
        for (let i = 0; i < runs.length; i++) {
          if (Math.abs(_size[i] * moduleSize - runs[i]) >= _size[i] * variance)
            return false;
        }
        return true;
      },
      add(out, x, y, total) {
        const moduleSize = total / FINDER.totalSize;
        const cur = { x, y, moduleSize, count: 1 };
        for (let idx = 0; idx < out.length; idx++) {
          const f = out[idx];
          if (!patternEquals(f, cur))
            continue;
          return out[idx] = patternMerge(f, cur);
        }
        out.push(cur);
        return;
      },
      toCenter(runs, end) {
        for (let i = p.length - 1; i > res.center; i--)
          end -= runs[i];
        end -= runs[res.center] / 2;
        return end;
      },
      check(b, runs, center, incr, maxCount) {
        let j = 0;
        let i = pointClone(center);
        const neg = pointNeg(incr);
        const check = (p2, step) => {
          for (; b.isInside(i) && !!b.point(i) === res.pattern[p2]; pointIncr(i, step)) {
            runs[p2]++;
            j++;
          }
          if (runs[p2] === 0)
            return true;
          const center2 = p2 === res.center;
          if (maxCount && !center2 && runs[p2] > res.size[p2] * maxCount)
            return true;
          return false;
        };
        for (let p2 = res.center; p2 >= 0; p2--)
          if (check(p2, neg))
            return false;
        i = pointClone(center);
        pointIncr(i, incr);
        j = 1;
        for (let p2 = res.center; p2 < res.length; p2++)
          if (check(p2, incr))
            return false;
        return j;
      },
      scanLine(b, y, xStart, xEnd, fn) {
        const runs = res.runs();
        let pos = 0;
        let x = xStart;
        if (xStart)
          while (x < xEnd && !!b.data[y][x] === res.pattern[0])
            x++;
        for (; x < xEnd; x++) {
          if (!!b.data[y][x] === res.pattern[pos]) {
            runs[pos]++;
            if (x !== b.width - 1)
              continue;
            x++;
          }
          if (pos !== res.length - 1) {
            runs[++pos]++;
            continue;
          }
          const found = fn(runs, x);
          if (found) {
            pos = 0;
            runs.fill(0);
          } else if (found === false) {
            break;
          } else {
            res.shift(runs, 2);
            pos = res.length - 2;
            runs[pos]++;
          }
        }
      }
    };
    return res;
  }
  var FINDER = pattern([true, false, true, false, true], [1, 1, 3, 1, 1]);
  var ALIGNMENT = pattern([false, true, false]);
  function findFinder(b) {
    let found = [];
    function checkRuns(runs, v = 2) {
      const total = sum(runs);
      if (total < FINDER.totalSize)
        return false;
      const moduleSize = total / FINDER.totalSize;
      return FINDER.checkSize(runs, moduleSize, v);
    }
    function checkLine(center, maxCount, total, incr) {
      const runs = FINDER.runs();
      let i = FINDER.check(b, runs, center, incr, maxCount);
      if (i === false)
        return false;
      const runsTotal = sum(runs);
      if (5 * Math.abs(runsTotal - total) >= 2 * total)
        return false;
      if (checkRuns(runs))
        return FINDER.toCenter(runs, i);
      return false;
    }
    function check(runs, i, j) {
      if (!checkRuns(runs))
        return false;
      const total = sum(runs);
      let x = FINDER.toCenter(runs, j);
      let y = checkLine({ x: int(x), y: i }, runs[2], total, { y: 1, x: 0 });
      if (y === false)
        return false;
      y += i;
      let xx = checkLine({ x: int(x), y: int(y) }, runs[2], total, { y: 0, x: 1 });
      if (xx === false)
        return false;
      x = xx + int(x);
      const dRuns = FINDER.runs();
      if (!FINDER.check(b, dRuns, { x: int(x), y: int(y) }, { x: 1, y: 1 }))
        return false;
      if (!checkRuns(dRuns, PATTERN_VARIANCE_DIAGONAL))
        return false;
      FINDER.add(found, x, y, total);
      return true;
    }
    let skipped = false;
    let ySkip = cap(int(3 * b.height / (4 * 97)), DETECT_MIN_ROW_SKIP);
    let done = false;
    for (let y = ySkip - 1; y < b.height && !done; y += ySkip) {
      FINDER.scanLine(b, y, 0, b.width, (runs, x) => {
        if (!check(runs, y, x))
          return;
        ySkip = 2;
        if (skipped) {
          let count = 0;
          let total = 0;
          for (const p3 of found) {
            if (p3.count < PATTERN_MIN_CONFIRMATIONS)
              continue;
            count++;
            total += p3.moduleSize;
          }
          if (count < 3)
            return;
          const average = total / found.length;
          let deviation = 0;
          for (const p3 of found)
            deviation += Math.abs(p3.moduleSize - average);
          if (deviation <= 0.05 * total) {
            done = true;
            return false;
          }
        } else if (found.length > 1) {
          const q = patternsConfirmed(found);
          if (q.length < 2)
            return true;
          skipped = true;
          const d = int((Math.abs(q[0].x - q[1].x) - Math.abs(q[0].y - q[1].y)) / 2);
          if (d <= runs[2] + ySkip)
            return true;
          y += d - runs[2] - ySkip;
          return false;
        }
        return;
      });
    }
    const flen = found.length;
    if (flen < 3)
      throw new Error(`Finder: len(found) = ${flen}`);
    found.sort((i, j) => i.moduleSize - j.moduleSize);
    const pBest = best2();
    for (let i = 0; i < flen - 2; i++) {
      const fi = found[i];
      for (let j = i + 1; j < flen - 1; j++) {
        const fj = found[j];
        const square0 = distance2(fi, fj);
        for (let k = j + 1; k < flen; k++) {
          const fk = found[k];
          if (fk.moduleSize > fi.moduleSize * 1.4)
            continue;
          const arr = [square0, distance2(fj, fk), distance2(fi, fk)].sort((a2, b3) => a2 - b3);
          const a = arr[0];
          const b2 = arr[1];
          const c = arr[2];
          pBest.add(Math.abs(c - 2 * b2) + Math.abs(c - 2 * a), [fi, fj, fk]);
        }
      }
    }
    const p = pBest.get();
    if (!p)
      throw new Error("cannot find finder");
    const p0 = p[0];
    const p1 = p[1];
    const p2 = p[2];
    const d01 = distance(p0, p1);
    const d12 = distance(p1, p2);
    const d02 = distance(p0, p2);
    let tl = p2;
    let bl = p0;
    let tr = p1;
    if (d12 >= d01 && d12 >= d02) {
      tl = p0;
      bl = p1;
      tr = p2;
    } else if (d02 >= d12 && d02 >= d01) {
      tl = p1;
      bl = p0;
      tr = p2;
    }
    if ((tr.x - tl.x) * (bl.y - tl.y) - (tr.y - tl.y) * (bl.x - tl.x) < 0) {
      let _bl = bl;
      bl = tr;
      tr = _bl;
    }
    return { bl, tl, tr };
  }
  function findAlignment(b, est, allowanceFactor) {
    const { moduleSize } = est;
    const allowance = int(allowanceFactor * moduleSize);
    const leftX = cap(est.x - allowance, 0);
    const rightX = cap(est.x + allowance, void 0, b.width - 1);
    const x = rightX - leftX;
    const topY = cap(est.y - allowance, 0);
    const bottomY = cap(est.y + allowance, void 0, b.height - 1);
    const y = bottomY - topY;
    if (x < moduleSize * 3 || y < moduleSize * 3)
      throw new Error(`x = ${x}, y=${y} moduleSize = ${moduleSize}`);
    const xStart = leftX;
    const yStart = topY;
    const width = rightX - leftX;
    const height = bottomY - topY;
    const found = [];
    const xEnd = xStart + width;
    const middleY = int(yStart + height / 2);
    for (let yGen = 0; yGen < height; yGen++) {
      const diff = int((yGen + 1) / 2);
      const y2 = middleY + (yGen & 1 ? -diff : diff);
      let res;
      ALIGNMENT.scanLine(b, y2, xStart, xEnd, (runs, x2) => {
        if (!ALIGNMENT.checkSize(runs, moduleSize))
          return;
        const total = sum(runs);
        const xx = ALIGNMENT.toCenter(runs, x2);
        const rVert = ALIGNMENT.runs();
        let v = ALIGNMENT.check(b, rVert, { x: int(xx), y: y2 }, { y: 1, x: 0 }, 2 * runs[1]);
        if (v === false)
          return;
        v += y2;
        const vTotal = sum(rVert);
        if (5 * Math.abs(vTotal - total) >= 2 * total)
          return;
        if (!ALIGNMENT.checkSize(rVert, moduleSize))
          return;
        const yy = ALIGNMENT.toCenter(rVert, v);
        res = ALIGNMENT.add(found, xx, yy, total);
        if (res)
          return false;
        return;
      });
      if (res)
        return res;
    }
    if (found.length > 0)
      return found[0];
    throw new Error("Alignment pattern not found");
  }
  function _single(b, from, to) {
    let steep = false;
    let d = { x: Math.abs(to.x - from.x), y: Math.abs(to.y - from.y) };
    if (d.y > d.x) {
      steep = true;
      from = pointMirror(from);
      to = pointMirror(to);
      d = pointMirror(d);
    }
    let error = -d.x / 2;
    let step = { x: from.x >= to.x ? -1 : 1, y: from.y >= to.y ? -1 : 1 };
    let runPos = 0;
    let xLimit = to.x + step.x;
    for (let x = from.x, y = from.y; x !== xLimit; x += step.x) {
      let real = { x, y };
      if (steep)
        real = pointMirror(real);
      if (runPos === 1 === !!b.point(real)) {
        if (runPos === 2)
          return distance({ x, y }, from);
        runPos++;
      }
      error += d.y;
      if (error <= 0)
        continue;
      if (y === to.y)
        break;
      y += step.y;
      error -= d.x;
    }
    if (runPos === 2)
      return distance({ x: to.x + step.x, y: to.y }, from);
    return NaN;
  }
  function BWBRunLength(b, from, to) {
    let result = _single(b, from, to);
    let scaleY = 1;
    const { x: fx, y: fy } = from;
    let otherToX = fx - (to.x - fx);
    const bw = b.width;
    if (otherToX < 0) {
      scaleY = fx / (fx - otherToX);
      otherToX = 0;
    } else if (otherToX >= bw) {
      scaleY = (bw - 1 - fx) / (otherToX - fx);
      otherToX = bw - 1;
    }
    let otherToY = int(fy - (to.y - fy) * scaleY);
    let scaleX = 1;
    const bh = b.height;
    if (otherToY < 0) {
      scaleX = fy / (fy - otherToY);
      otherToY = 0;
    } else if (otherToY >= bh) {
      scaleX = (bh - 1 - fy) / (otherToY - fy);
      otherToY = bh - 1;
    }
    otherToX = int(fx + (otherToX - fx) * scaleX);
    result += _single(b, from, { x: otherToX, y: otherToY });
    return result - 1;
  }
  function moduleSizeAvg(b, p1, p2) {
    const est1 = BWBRunLength(b, pointInt(p1), pointInt(p2));
    const est2 = BWBRunLength(b, pointInt(p2), pointInt(p1));
    if (Number.isNaN(est1))
      return est2 / FINDER.totalSize;
    if (Number.isNaN(est2))
      return est1 / FINDER.totalSize;
    return (est1 + est2) / (2 * FINDER.totalSize);
  }
  function detect(b) {
    const { bl, tl, tr } = findFinder(b);
    const moduleSize = (moduleSizeAvg(b, tl, tr) + moduleSizeAvg(b, tl, bl)) / 2;
    if (moduleSize < 1)
      throw new Error(`invalid moduleSize = ${moduleSize}`);
    const tltr = int(distance(tl, tr) / moduleSize + 0.5);
    const tlbl = int(distance(tl, bl) / moduleSize + 0.5);
    let size = int((tltr + tlbl) / 2 + 7);
    const rem = size % 4;
    if (rem === 0)
      size++;
    else if (rem === 2)
      size--;
    else if (rem === 3)
      size -= 2;
    const version = info2.size.decode(size);
    validateVersion2(version);
    let alignmentPattern;
    if (info2.alignmentPatterns(version).length > 0) {
      const br2 = { x: tr.x - tl.x + bl.x, y: tr.y - tl.y + bl.y };
      const c = 1 - 3 / (info2.size.encode(version) - 7);
      const est = {
        x: int(tl.x + c * (br2.x - tl.x)),
        y: int(tl.y + c * (br2.y - tl.y)),
        moduleSize,
        count: 1
      };
      for (let i = 4; i <= 16; i <<= 1) {
        try {
          alignmentPattern = findAlignment(b, est, i);
          break;
        } catch (e) {
        }
      }
    }
    const toTL = { x: 3.5, y: 3.5 };
    const toTR = { x: size - 3.5, y: 3.5 };
    const toBL = { x: 3.5, y: size - 3.5 };
    let br;
    let toBR;
    if (alignmentPattern) {
      br = alignmentPattern;
      toBR = { x: size - 6.5, y: size - 6.5 };
    } else {
      br = { x: tr.x - tl.x + bl.x, y: tr.y - tl.y + bl.y };
      toBR = { x: size - 3.5, y: size - 3.5 };
    }
    const from = [tl, tr, br, bl];
    const bits = transform(b, size, from, [toTL, toTR, toBR, toBL]);
    return { bits, points: from };
  }
  function squareToQuadrilateral(p) {
    const d3 = { x: p[0].x - p[1].x + p[2].x - p[3].x, y: p[0].y - p[1].y + p[2].y - p[3].y };
    if (d3.x === 0 && d3.y === 0) {
      return [
        [p[1].x - p[0].x, p[2].x - p[1].x, p[0].x],
        [p[1].y - p[0].y, p[2].y - p[1].y, p[0].y],
        [0, 0, 1]
      ];
    } else {
      const d1 = { x: p[1].x - p[2].x, y: p[1].y - p[2].y };
      const d2 = { x: p[3].x - p[2].x, y: p[3].y - p[2].y };
      const den = d1.x * d2.y - d2.x * d1.y;
      const p13 = (d3.x * d2.y - d2.x * d3.y) / den;
      const p23 = (d1.x * d3.y - d3.x * d1.y) / den;
      return [
        [p[1].x - p[0].x + p13 * p[1].x, p[3].x - p[0].x + p23 * p[3].x, p[0].x],
        [p[1].y - p[0].y + p13 * p[1].y, p[3].y - p[0].y + p23 * p[3].y, p[0].y],
        [p13, p23, 1]
      ];
    }
  }
  function transform(b, size, from, to) {
    const p = squareToQuadrilateral(to);
    const qToS = [
      [
        p[1][1] * p[2][2] - p[2][1] * p[1][2],
        p[2][1] * p[0][2] - p[0][1] * p[2][2],
        p[0][1] * p[1][2] - p[1][1] * p[0][2]
      ],
      [
        p[2][0] * p[1][2] - p[1][0] * p[2][2],
        p[0][0] * p[2][2] - p[2][0] * p[0][2],
        p[1][0] * p[0][2] - p[0][0] * p[1][2]
      ],
      [
        p[1][0] * p[2][1] - p[2][0] * p[1][1],
        p[2][0] * p[0][1] - p[0][0] * p[2][1],
        p[0][0] * p[1][1] - p[1][0] * p[0][1]
      ]
    ];
    const sToQ = squareToQuadrilateral(from);
    const transform2 = sToQ.map((i) => i.map((_, qx) => i.reduce((acc, v, j) => acc + v * qToS[j][qx], 0)));
    const res = new Bitmap(size);
    const points = fillArr2(2 * size, 0);
    const pointsLength = points.length;
    for (let y = 0; y < size; y++) {
      const p2 = transform2;
      for (let i = 0; i < pointsLength - 1; i += 2) {
        const x = i / 2 + 0.5;
        const y2 = y + 0.5;
        const den = p2[2][0] * x + p2[2][1] * y2 + p2[2][2];
        points[i] = int((p2[0][0] * x + p2[0][1] * y2 + p2[0][2]) / den);
        points[i + 1] = int((p2[1][0] * x + p2[1][1] * y2 + p2[1][2]) / den);
      }
      for (let i = 0; i < pointsLength; i += 2) {
        const px = cap(points[i], 0, b.width - 1);
        const py = cap(points[i + 1], 0, b.height - 1);
        if (b.data[py][px])
          res.data[y][i / 2] = true;
      }
    }
    return res;
  }
  function readInfoBits(b) {
    const readBit = (x, y, out) => out << 1 | (b.data[y][x] ? 1 : 0);
    const size = b.height;
    let version1 = 0;
    for (let y = 5; y >= 0; y--)
      for (let x = size - 9; x >= size - 11; x--)
        version1 = readBit(x, y, version1);
    let version2 = 0;
    for (let x = 5; x >= 0; x--)
      for (let y = size - 9; y >= size - 11; y--)
        version2 = readBit(x, y, version2);
    let format1 = 0;
    for (let x = 0; x < 6; x++)
      format1 = readBit(x, 8, format1);
    format1 = readBit(7, 8, format1);
    format1 = readBit(8, 8, format1);
    format1 = readBit(8, 7, format1);
    for (let y = 5; y >= 0; y--)
      format1 = readBit(8, y, format1);
    let format2 = 0;
    for (let y = size - 1; y >= size - 7; y--)
      format2 = readBit(8, y, format2);
    for (let x = size - 8; x < size; x++)
      format2 = readBit(x, 8, format2);
    return { version1, version2, format1, format2 };
  }
  function parseInfo(b) {
    const popcnt = (a) => {
      let cnt = 0;
      while (a) {
        if (a & 1)
          cnt++;
        a >>= 1;
      }
      return cnt;
    };
    const size = b.height;
    const { version1, version2, format1, format2 } = readInfoBits(b);
    let format;
    const bestFormat = best2();
    for (const ecc of ["medium", "low", "high", "quartile"]) {
      for (let mask = 0; mask < 8; mask++) {
        const bits = info2.formatBits(ecc, mask);
        const cur = { ecc, mask };
        if (bits === format1 || bits === format2) {
          format = cur;
          break;
        }
        bestFormat.add(popcnt(format1 ^ bits), cur);
        if (format1 !== format2)
          bestFormat.add(popcnt(format2 ^ bits), cur);
      }
    }
    if (format === void 0 && bestFormat.score() <= MAX_BITS_ERROR)
      format = bestFormat.get();
    if (format === void 0)
      throw new Error("invalid format pattern");
    let version = info2.size.decode(size);
    if (version < 7)
      validateVersion2(version);
    else {
      version = void 0;
      const bestVer = best2();
      for (let ver = 7; ver <= 40; ver++) {
        const bits = info2.versionBits(ver);
        if (bits === version1 || bits === version2) {
          version = ver;
          break;
        }
        bestVer.add(popcnt(version1 ^ bits), ver);
        if (version1 !== version2)
          bestVer.add(popcnt(version2 ^ bits), ver);
      }
      if (version === void 0 && bestVer.score() <= MAX_BITS_ERROR)
        version = bestVer.get();
      if (version === void 0)
        throw new Error("invalid version pattern");
      if (info2.size.encode(version) !== size)
        throw new Error("invalid version size");
    }
    return { version, ...format };
  }
  function decodeBitmap(b) {
    const size = b.height;
    if (size < 21 || (size & 3) !== 1 || size !== b.width)
      throw new Error(`decode: invalid size=${size}`);
    const { version, mask, ecc } = parseInfo(b);
    const tpl = drawTemplate2(version, ecc, mask);
    const { total } = info2.capacity(version, ecc);
    const bytes = new Uint8Array(total);
    let pos = 0;
    let buf = 0;
    let bitPos = 0;
    zigzag2(tpl, mask, (x, y, m) => {
      bitPos++;
      buf <<= 1;
      buf |= +(!!b.data[y][x] !== m);
      if (bitPos !== 8)
        return;
      bytes[pos++] = buf;
      bitPos = 0;
      buf = 0;
    });
    if (pos !== total)
      throw new Error(`decode: pos=${pos}, total=${total}`);
    let bits = Array.from(interleave2(version, ecc).decode(bytes)).map((i) => bin2(i, 8)).join("");
    const readBits = (n) => {
      if (n > bits.length)
        throw new Error("Not enough bits");
      const val = bits.slice(0, n);
      bits = bits.slice(n);
      return val;
    };
    const toNum = (n) => Number(`0b${n}`);
    const modes = {
      "0000": "terminator",
      "0001": "numeric",
      "0010": "alphanumeric",
      "0100": "byte",
      "0111": "eci",
      "1000": "kanji"
    };
    let res = "";
    while (true) {
      if (bits.length < 4)
        break;
      const modeBits = readBits(4);
      const mode = modes[modeBits];
      if (mode === void 0)
        throw new Error(`Unknown modeBits=${modeBits} res="${res}"`);
      if (mode === "terminator")
        break;
      const countBits = info2.lengthBits(version, mode);
      let count = toNum(readBits(countBits));
      if (mode === "numeric") {
        while (count >= 3) {
          const v = toNum(readBits(10));
          if (v >= 1e3)
            throw new Error(`numberic(3) = ${v}`);
          res += v.toString().padStart(3, "0");
          count -= 3;
        }
        if (count === 2) {
          const v = toNum(readBits(7));
          if (v >= 100)
            throw new Error(`numeric(2) = ${v}`);
          res += v.toString().padStart(2, "0");
        } else if (count === 1) {
          const v = toNum(readBits(4));
          if (v >= 10)
            throw new Error(`Numeric(1) = ${v}`);
          res += v.toString();
        }
      } else if (mode === "alphanumeric") {
        while (count >= 2) {
          const v = toNum(readBits(11));
          res += info2.alphabet.alphanumerc.encode([Math.floor(v / 45), v % 45]).join("");
          count -= 2;
        }
        if (count === 1)
          res += info2.alphabet.alphanumerc.encode([toNum(readBits(6))]).join("");
      } else if (mode === "byte") {
        let utf8 = [];
        for (let i = 0; i < count; i++)
          utf8.push(Number(`0b${readBits(8)}`));
        res += new TextDecoder().decode(new Uint8Array(utf8));
      } else
        throw new Error(`Unknown mode=${mode}`);
    }
    return res;
  }
  function cropToSquare(img) {
    const data = Array.isArray(img.data) ? new Uint8Array(img.data) : img.data;
    const { height, width } = img;
    const squareSize = Math.min(height, width);
    const offset = {
      x: Math.floor((width - squareSize) / 2),
      y: Math.floor((height - squareSize) / 2)
    };
    const bytesPerPixel = getBytesPerPixel(img);
    const croppedData = new Uint8Array(squareSize * squareSize * bytesPerPixel);
    for (let y = 0; y < squareSize; y++) {
      const srcPos = ((y + offset.y) * width + offset.x) * bytesPerPixel;
      const dstPos = y * squareSize * bytesPerPixel;
      const length = squareSize * bytesPerPixel;
      croppedData.set(data.subarray(srcPos, srcPos + length), dstPos);
    }
    return { offset, img: { height: squareSize, width: squareSize, data: croppedData } };
  }
  function decodeQR(img, opts = {}) {
    for (const field of ["height", "width"]) {
      if (!Number.isSafeInteger(img[field]) || img[field] <= 0)
        throw new Error(`invalid img.${field}=${img[field]} (${typeof img[field]})`);
    }
    if (!Array.isArray(img.data) && !(img.data instanceof Uint8Array) && !(img.data instanceof Uint8ClampedArray))
      throw new Error(`invalid image.data=${img.data} (${typeof img.data})`);
    if (opts.cropToSquare !== void 0 && typeof opts.cropToSquare !== "boolean")
      throw new Error(`invalid opts.cropToSquare=${opts.cropToSquare}`);
    for (const fn of ["pointsOnDetect", "imageOnBitmap", "imageOnDetect", "imageOnResult"]) {
      if (opts[fn] !== void 0 && typeof opts[fn] !== "function")
        throw new Error(`invalid opts.${fn}=${opts[fn]} (${typeof opts[fn]})`);
    }
    let offset = { x: 0, y: 0 };
    if (opts.cropToSquare)
      ({ img, offset } = cropToSquare(img));
    const bmp = toBitmap(img);
    if (opts.imageOnBitmap)
      opts.imageOnBitmap(bmp.toImage());
    const { bits, points } = detect(bmp);
    if (opts.pointsOnDetect) {
      const p = points.map((i) => ({ ...i, ...pointAdd(i, offset) }));
      opts.pointsOnDetect(p);
    }
    if (opts.imageOnDetect)
      opts.imageOnDetect(bits.toImage());
    const res = decodeBitmap(bits);
    if (opts.imageOnResult)
      opts.imageOnResult(bits.toImage());
    return res;
  }
  var decode_default = decodeQR;

  // ../../esm/dom.js
  var dom_exports = {};
  __export(dom_exports, {
    QRCanvas: () => QRCanvas,
    frameLoop: () => frameLoop,
    frontalCamera: () => frontalCamera,
    getSize: () => getSize,
    svgToPng: () => svgToPng
  });
  var getSize = (elm) => {
    const css = getComputedStyle(elm);
    const width = Math.floor(+css.width.split("px")[0]);
    const height = Math.floor(+css.height.split("px")[0]);
    return { width, height };
  };
  var setCanvasSize = (canvas, height, width) => {
    if (canvas.height !== height)
      canvas.height = height;
    if (canvas.width !== width)
      canvas.width = width;
  };
  var getCanvasContext = (canvas) => {
    const context = canvas.getContext("2d");
    if (context === null)
      throw new Error("Cannot get canvas context");
    return { canvas, context };
  };
  var clearCanvas = ({ canvas, context }) => {
    context.clearRect(0, 0, canvas.width, canvas.height);
  };
  var QRCanvas = class {
    constructor({ overlay, bitmap, resultQR } = {}, opts = {}) {
      this.lastDetect = 0;
      this.opts = {
        resultBlockSize: 8,
        overlayMainColor: "green",
        overlayFinderColor: "blue",
        overlaySideColor: "black",
        overlayTimeout: 500,
        cropToSquare: true,
        ...opts
      };
      this.main = getCanvasContext(document.createElement("canvas"));
      if (overlay)
        this.overlay = getCanvasContext(overlay);
      if (bitmap)
        this.bitmap = getCanvasContext(bitmap);
      if (resultQR) {
        this.resultQR = getCanvasContext(resultQR);
        this.resultQR.context.imageSmoothingEnabled = false;
      }
    }
    setSize(height, width) {
      setCanvasSize(this.main.canvas, height, width);
      if (this.overlay)
        setCanvasSize(this.overlay.canvas, height, width);
      if (this.bitmap)
        setCanvasSize(this.bitmap.canvas, height, width);
    }
    drawBitmap({ data, height, width }) {
      if (!this.bitmap)
        return;
      const imgData = new ImageData(Uint8ClampedArray.from(data), width, height);
      let offset = { x: 0, y: 0 };
      if (this.opts.cropToSquare) {
        offset = {
          x: Math.ceil((this.bitmap.canvas.width - width) / 2),
          y: Math.ceil((this.bitmap.canvas.height - height) / 2)
        };
      }
      this.bitmap.context.putImageData(imgData, offset.x, offset.y);
    }
    drawResultQr({ data, height, width }) {
      if (!this.resultQR)
        return;
      const blockSize = this.opts.resultBlockSize;
      setCanvasSize(this.resultQR.canvas, height, width);
      const imgData = new ImageData(Uint8ClampedArray.from(data), width, height);
      this.resultQR.context.putImageData(imgData, 0, 0);
      this.resultQR.canvas.style = `image-rendering: pixelated; width: ${blockSize * width}px; height: ${blockSize * height}px`;
    }
    drawOverlay(points) {
      if (!this.overlay)
        return;
      const ctx = this.overlay.context;
      const height = this.overlay.canvas.height;
      const width = this.overlay.canvas.width;
      if (this.opts.cropToSquare && height !== width) {
        const squareSize = Math.min(height, width);
        const offset = {
          x: Math.floor((width - squareSize) / 2),
          y: Math.floor((height - squareSize) / 2)
        };
        ctx.clearRect(offset.x, offset.y, squareSize, squareSize);
        ctx.fillStyle = this.opts.overlaySideColor;
        if (width > height) {
          ctx.fillRect(0, 0, offset.x, height);
          ctx.fillRect(width - offset.x, 0, offset.x, height);
        } else if (height > width) {
          ctx.fillRect(0, 0, width, offset.y);
          ctx.fillRect(0, height - offset.y, width, offset.y);
        }
      } else {
        ctx.clearRect(0, 0, width, height);
      }
      if (points) {
        const [tl, tr, br, bl] = points;
        ctx.fillStyle = this.opts.overlayMainColor;
        ctx.beginPath();
        ctx.moveTo(tl.x, tl.y);
        ctx.lineTo(tr.x, tr.y);
        ctx.lineTo(br.x, br.y);
        ctx.lineTo(bl.x, bl.y);
        ctx.fill();
        ctx.closePath();
        ctx.fillStyle = this.opts.overlayFinderColor;
        for (const p of points) {
          if (!("moduleSize" in p))
            continue;
          const x = p.x - 3 * p.moduleSize;
          const y = p.y - 3 * p.moduleSize;
          const size = 7 * p.moduleSize;
          ctx.fillRect(x, y, size, size);
        }
      }
    }
    drawImage(image, height, width) {
      this.setSize(height, width);
      const { context } = this.main;
      context.drawImage(image, 0, 0, width, height);
      const data = context.getImageData(0, 0, width, height);
      const options = { cropToSquare: this.opts.cropToSquare };
      if (this.bitmap)
        options.imageOnBitmap = (img) => this.drawBitmap(img);
      if (this.overlay)
        options.pointsOnDetect = (points) => this.drawOverlay(points);
      if (this.resultQR)
        options.imageOnResult = (img) => this.drawResultQr(img);
      try {
        const res = decode_default(data, options);
        this.lastDetect = Date.now();
        return res;
      } catch (e) {
        if (this.overlay && Date.now() - this.lastDetect > this.opts.overlayTimeout)
          this.drawOverlay();
      }
      return;
    }
    clear() {
      clearCanvas(this.main);
      if (this.overlay)
        clearCanvas(this.overlay);
      if (this.bitmap)
        clearCanvas(this.bitmap);
      if (this.resultQR)
        clearCanvas(this.resultQR);
    }
  };
  var QRCamera = class {
    constructor(stream, player) {
      this.stream = stream;
      this.player = player;
      this.setStream(stream);
    }
    setStream(stream) {
      this.stream = stream;
      const { player } = this;
      player.setAttribute("autoplay", "");
      player.setAttribute("muted", "");
      player.setAttribute("playsinline", "");
      player.srcObject = stream;
    }
    /**
     * Returns list of cameras
     * NOTE: available only after first getUserMedia request, so cannot be additional method
     */
    async listDevices() {
      if (!navigator.mediaDevices || !navigator.mediaDevices.enumerateDevices)
        throw new Error("Media Devices not supported");
      const devices = await navigator.mediaDevices.enumerateDevices();
      return devices.filter((device) => device.kind === "videoinput").map((i) => ({
        deviceId: i.deviceId,
        label: i.label || `Camera ${i.deviceId}`
      }));
    }
    /**
     * Change stream to different camera
     * @param deviceId - devideId from '.listDevices'
     */
    async setDevice(deviceId) {
      this.stop();
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { deviceId: { exact: deviceId } }
      });
      this.setStream(stream);
    }
    readFrame(canvas, fullSize = false) {
      const { player } = this;
      if (fullSize)
        return canvas.drawImage(player, player.videoHeight, player.videoWidth);
      const size = getSize(player);
      return canvas.drawImage(player, size.height, size.width);
    }
    stop() {
      for (const track of this.stream.getTracks())
        track.stop();
    }
  };
  async function frontalCamera(player) {
    const stream = await navigator.mediaDevices.getUserMedia({
      video: {
        // Ask for screen resolution
        height: { ideal: window.screen.height },
        width: { ideal: window.screen.width },
        // prefer front-facing camera, but can use any other
        // NOTE: 'exact' will cause OverConstrained error if no frontal camera available
        facingMode: "environment"
      }
    });
    return new QRCamera(stream, player);
  }
  function frameLoop(cb) {
    let handle = void 0;
    function loop(ts) {
      cb(ts);
      handle = requestAnimationFrame(loop);
    }
    handle = requestAnimationFrame(loop);
    return () => {
      if (handle === void 0)
        return;
      cancelAnimationFrame(handle);
      handle = void 0;
    };
  }
  function svgToPng(svgData, width, height) {
    return new Promise((resolve, reject) => {
      if (!(Number.isSafeInteger(width) && Number.isSafeInteger(height) && width > 0 && height > 0 && width < 8192 && height < 8192))
        return reject(new Error("invalid width and height: " + width + " " + height));
      const domparser = new DOMParser();
      const doc = domparser.parseFromString(svgData, "image/svg+xml");
      const svgElement = doc.documentElement;
      const rect = doc.createElementNS("http://www.w3.org/2000/svg", "rect");
      rect.setAttribute("width", "100%");
      rect.setAttribute("height", "100%");
      rect.setAttribute("fill", "white");
      svgElement.insertBefore(rect, svgElement.firstChild);
      const serializer = new XMLSerializer();
      const source = serializer.serializeToString(doc);
      const img = new Image();
      img.src = "data:image/svg+xml," + encodeURIComponent(source);
      img.onload = function() {
        const canvas = document.createElement("canvas");
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext("2d");
        if (!ctx)
          return reject(new Error("was not able to create 2d context"));
        ctx.drawImage(img, 0, 0, width, height);
        const dataUrl = canvas.toDataURL("image/png");
        resolve(dataUrl);
      };
      img.onerror = reject;
    });
  }
  return __toCommonJS(input_exports);
})();
/*!
Copyright (c) 2023 Paul Miller (paulmillr.com)
The library paulmillr-qr is dual-licensed under the Apache 2.0 OR MIT license.
You can select a license of your choice.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

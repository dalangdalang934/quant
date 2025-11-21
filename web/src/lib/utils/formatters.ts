import { format } from "date-fns";
import numeral from "numeral";

const usdFormatter = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

const truncateTo = (value: number, decimals = 2) => {
  const factor = 10 ** decimals;
  if (value >= 0) {
    return Math.floor(value * factor) / factor;
  }
  return Math.ceil(value * factor) / factor;
};

export const fmtUSD = (n?: number | null) => {
  if (n == null || Number.isNaN(n)) return "--";
  if (Math.abs(n) < 1e-8) return "$0.00";
  // 对于小额盈亏（<$1），显示更多小数位以保持精度
  if (Math.abs(n) < 1) {
    return `$${n >= 0 ? '' : '-'}${Math.abs(n).toFixed(4)}`;
  }
  // 对于较大金额，保持2位小数
  return usdFormatter.format(n);
};

export const fmtPct = (n?: number | null) =>
  n == null ? "--" : numeral(n).format("0.00%");

export const fmtInt = (n?: number | null) =>
  n == null ? "--" : numeral(n).format("0,0");

// 清理 HTML 标签（如 <br/>, <br>, <b>, </b> 等）
export const stripHtmlTags = (text: string | null | undefined): string => {
  if (!text) return "";
  return text
    .replace(/<[^>]*>/g, " ") // 替换所有 HTML 标签为空格
    .replace(/\s+/g, " ")     // 多个空格合并为一个
    .trim();                  // 去除首尾空格
};

export const fmtNumber = (n?: number | null, decimals: number = 2) => {
  if (n == null || Number.isNaN(n)) return "--";
  return n.toFixed(decimals);
};

export const fmtTs = (unixSeconds?: number | null) =>
  unixSeconds == null
    ? "--"
    : format(unixSeconds * 1000, "yyyy-MM-dd HH:mm:ss");

export const pnlClass = (n?: number | null) =>
  n == null || Number.isNaN(n)
    ? "text-zinc-300"
    : n > 0
      ? "text-green-400"
      : n < 0
        ? "text-red-400"
        : "text-zinc-300";

export const withSign = (n?: number | null, digits = 2) =>
  n == null
    ? "--"
    : `${n > 0 ? "+" : n < 0 ? "-" : ""}${Math.abs(n).toFixed(digits)}`;

// 专门用于显示价格，保持交易所级别的精度
export const fmtPrice = (n?: number | null) => {
  if (n == null || Number.isNaN(n)) return "--";
  // 对于特别大的数字（如BTC价格）使用2位小数
  // 对于一般价格使用4位小数以保持精度
  const abs = Math.abs(n);
  const digits = abs >= 10000 ? 2 : 4;
  return n.toFixed(digits);
};

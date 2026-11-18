// Evaluator for the expression IR produced by internal/expr (Go).
import type { IR, Value } from "./types";

export class EvalError extends Error {}

/** Rounds half away from zero, like Go's math.Round. */
export function round(x: number): number {
  return x < 0 ? -Math.round(-x) : Math.round(x);
}

export function roundTo(x: number, decimals: number): number {
  const p = 10 ** decimals;
  return round(x * p) / p;
}

/** Prints a number rounded to at most two decimals (matches Go FormatNumber). */
export function formatNumber(x: number): string {
  const r = roundTo(x, 2);
  return r === 0 ? "0" : String(r);
}

export function kind(v: Value): string {
  return typeof v === "number" ? "number" : typeof v === "string" ? "string" : "boolean";
}

export function str(v: Value): string {
  return typeof v === "number" ? formatNumber(v) : String(v);
}

export type Env = Record<string, Value>;

function finite(f: number): number {
  if (!Number.isFinite(f)) throw new EvalError("result is not a finite number");
  return f;
}

function num(n: IR, env: Env, what: string): number {
  const v = evaluate(n, env);
  if (typeof v !== "number") throw new EvalError(`${what} expects a number, got ${kind(v)} "${str(v)}"`);
  return v;
}

function bool(n: IR, env: Env, what: string): boolean {
  const v = evaluate(n, env);
  if (typeof v !== "boolean") throw new EvalError(`${what} expects true/false, got ${kind(v)} "${str(v)}"`);
  return v;
}

export function evaluate(n: IR, env: Env): Value {
  const op = n[0];
  const a = n as unknown[];
  switch (op) {
    case "n":
      return a[1] as number;
    case "s":
      return a[1] as string;
    case "b":
      return a[1] as boolean;
    case "v": {
      const name = a[1] as string;
      if (!(name in env)) throw new EvalError(`unknown variable "${name}"`);
      return env[name] as Value;
    }
    case "neg":
      return -num(a[1] as IR, env, "-");
    case "!":
      return !bool(a[1] as IR, env, "!");
    case "?":
      return bool(a[1] as IR, env, "?:") ? evaluate(a[2] as IR, env) : evaluate(a[3] as IR, env);
    case "&&":
    case "||": {
      const l = bool(a[1] as IR, env, op);
      if ((op === "&&" && !l) || (op === "||" && l)) return l;
      return bool(a[2] as IR, env, op);
    }
    case "==":
    case "!=": {
      const l = evaluate(a[1] as IR, env);
      const r = evaluate(a[2] as IR, env);
      if (typeof l !== typeof r) throw new EvalError(`cannot compare ${kind(l)} "${str(l)}" with ${kind(r)} "${str(r)}"`);
      return op === "==" ? l === r : l !== r;
    }
    case "+": {
      const l = evaluate(a[1] as IR, env);
      const r = evaluate(a[2] as IR, env);
      if (typeof l === "number" && typeof r === "number") return finite(l + r);
      if (typeof l === "string" || typeof r === "string") return str(l) + str(r);
      throw new EvalError(`cannot add ${kind(l)} and ${kind(r)}`);
    }
    case "-":
    case "*":
    case "/":
    case "%":
    case "^":
    case "<":
    case "<=":
    case ">":
    case ">=": {
      const l = num(a[1] as IR, env, op);
      const r = num(a[2] as IR, env, op);
      switch (op) {
        case "-":
          return finite(l - r);
        case "*":
          return finite(l * r);
        case "/":
          if (r === 0) throw new EvalError("division by zero");
          return finite(l / r);
        case "%":
          if (r === 0) throw new EvalError("modulo by zero");
          return finite(l % r);
        case "^":
          return finite(l ** r);
        case "<":
          return l < r;
        case "<=":
          return l <= r;
        case ">":
          return l > r;
        default:
          return l >= r;
      }
    }
    case "call":
      return call(a[1] as string, a.slice(2) as IR[], env);
  }
  throw new EvalError(`invalid expression node "${op}"`);
}

function call(name: string, args: IR[], env: Env): Value {
  switch (name) {
    case "if":
      return bool(args[0] as IR, env, "if()") ? evaluate(args[1] as IR, env) : evaluate(args[2] as IR, env);
    case "str":
      return str(evaluate(args[0] as IR, env));
    case "plural": {
      const c = num(args[0] as IR, env, "plural()");
      const one = str(evaluate(args[1] as IR, env));
      if (c === 1) return one;
      return args.length === 3 ? str(evaluate(args[2] as IR, env)) : `${one}s`;
    }
  }
  const x = args.map((a) => num(a, env, `${name}()`));
  const step = (fn: (v: number) => number): number => {
    if (x.length === 2) {
      const s = x[1] as number;
      if (s <= 0) throw new EvalError(`${name}() step must be positive`);
      return finite(fn((x[0] as number) / s) * s);
    }
    return finite(fn(x[0] as number));
  };
  const x0 = x[0] as number;
  switch (name) {
    case "min":
      return x.reduce((m, v) => (v < m ? v : m));
    case "max":
      return x.reduce((m, v) => (v > m ? v : m));
    case "abs":
      return Math.abs(x0);
    case "round":
      return step(round);
    case "floor":
      return step(Math.floor);
    case "ceil":
      return step(Math.ceil);
    case "clamp":
      return Math.min(Math.max(x0, x[1] as number), x[2] as number);
    case "sqrt":
      if (x0 < 0) throw new EvalError("sqrt() of a negative number");
      return Math.sqrt(x0);
    case "pow":
      return finite(x0 ** (x[1] as number));
    case "lerp":
      return finite(x0 + ((x[1] as number) - x0) * (x[2] as number));
  }
  throw new EvalError(`unknown function "${name}"`);
}

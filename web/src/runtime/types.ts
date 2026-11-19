// Types mirroring internal/recipe/compiled.go and internal/runtime/runtime.go.

export type IR = [string, ...unknown[]];

export type Segment =
  | string
  | { e: IR; f?: string; a?: string[] }
  | { ing: string; p?: IR; n?: boolean; pt?: string }
  | { eq: string }
  | { timer: IR }
  | { if: { c: IR; t: Template }[]; else?: Template };

export type Template = Segment[];

export interface Tier {
  below?: number;
  step: number;
}

export interface Rule {
  denoms?: number[];
  tiers?: Tier[];
  min?: number;
  max?: number;
  tiny?: string;
}

export interface Rung extends Rule {
  unit: string;
}

export interface Unit {
  id: string;
  dim: string;
  sys?: string;
  f: number;
  one: string;
  other: string;
}

export interface Table {
  units: Record<string, Unit>;
  ladders: Record<string, Rung[]>;
  tol: number;
  fractions: string;
  count: Rule;
  fallback: Rule;
  dec?: string;
  words?: Record<string, string>;
  of?: string;
  digits?: string;
  glyphs?: Record<string, string>;
}

export interface Ing {
  name: string;
  plural?: string;
  gpml?: number;
  each?: number;
  ug?: Record<string, number>;
  measure?: string;
  round?: Rule;
  cat?: string;
}

export interface Qty {
  v: number;
  h?: number;
  t: string;
  u: string;
  id: string;
}

export interface OptionC {
  id: string;
  type: "toggle" | "choice" | "number";
  label: string;
  help?: string;
  default: boolean | string | number;
  choices?: { id: string; label: string }[];
  min?: number;
  max?: number;
  step?: number;
  unit?: string;
  when?: IR;
}

export interface RecipeRef {
  slug: string;
  one: string;
  other: string;
  min: number;
  max: number;
  step: number;
  with?: { id: string; type: string; e: IR }[];
}

export interface IngC {
  id: string;
  ref?: string;
  base?: string;
  name: string;
  plural?: string;
  unit?: string;
  amount?: Amount;
  fixed?: boolean;
  when?: IR;
  note?: Template;
  optional?: boolean;
  recipe?: RecipeRef;
  split?: { id: string; a: Amount }[];
}

export interface Amount {
  n?: number;
  s?: boolean;
  e?: IR;
  hi?: Amount;
}

export interface Compiled {
  v: number;
  system: string;
  servings: { default: number; min: number; max: number; step: number; one: string; other: string };
  options?: OptionC[];
  scale?: IR;
  vars?: { id: string; e: IR }[];
  times?: { id: string; label: string; e: IR }[];
  equipment?: { id: string; when?: IR; name: Template; note?: Template }[];
  ingredients: { title?: string; when?: IR; items: IngC[] }[];
  steps: { title?: string; when?: IR; items: { when?: IR; text: Template; note?: Template; image?: string }[] }[];
  notes?: { when?: IR; kind: string; text: Template }[];
  ings?: Record<string, Ing>;
  units: Table;
  images?: { when: IR; src: string; srcset?: string; alt?: string }[];
}

export type Value = number | string | boolean;

export interface State {
  servings?: number;
  options?: Record<string, Value>;
  system?: string;
}

export interface LinkM {
  slug: string;
  servings: number;
  options: Record<string, Value>;
}

export interface Seg {
  t: "text" | "ing" | "eq" | "timer";
  v: string;
  id: string;
  q: string;
  j: string;
  min: number;
  link: LinkM | null;
}

export interface IngM {
  id: string;
  q: Qty | null;
  j: string;
  name: string;
  note: string;
  optional: boolean;
  link: LinkM | null;
  uses: { n: number; q: string }[];
}

export interface Model {
  servings: number;
  yield: string;
  scale: number;
  system: string;
  options: string[];
  times: { id: string; label: string; minutes: number; text: string }[];
  equipment: { id: string; name: string; note: string }[];
  ingredients: { title: string; items: IngM[] }[];
  steps: { title: string; items: { n: number; segs: Seg[]; note: string; image: string }[] }[];
  notes: { kind: string; text: string }[];
  image: { src: string; srcset: string; alt: string } | null;
}

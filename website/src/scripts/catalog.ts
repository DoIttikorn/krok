// Mirrors internal/core/catalog.go and the rules in internal/core/options.go.
export type Framework = "gin" | "echo" | "chi";
export type Database = "postgres" | "mysql" | "mongodb" | "none";

export const frameworks: Framework[] = ["gin", "echo", "chi"];
export const databases: Database[] = ["postgres", "mysql", "mongodb", "none"];

export interface Extra {
  id: string;
  flag: string;
  label: string;
  color: number;
  // Which port of the hexagon the adapter plugs into (see portSlots).
  slot: number;
}

// Direction of each hexagon face, in degrees around the domain core.
export const portSlots = {
  http: 180,
  repository: 0,
  docs: 120,
  config: 240,
  messaging: 60,
  jobs: 300,
} as const;

export const portLabels: Record<number, string> = {
  180: "HTTP port · driving",
  0: "Repository port · driven",
  120: "Docs port · driving",
  240: "Config port · driven",
  60: "Cache and messaging port · driven",
  300: "Jobs and events port · driven",
};

export const extras: Extra[] = [
  { id: "openapi", flag: "--openapi", label: "OpenAPI (Huma)", color: 0x86efac, slot: portSlots.docs },
  { id: "redis", flag: "--redis", label: "Redis", color: 0xf87171, slot: portSlots.messaging },
  { id: "kafka", flag: "--kafka", label: "Kafka", color: 0xe2e8f0, slot: portSlots.messaging },
  { id: "rabbitmq", flag: "--rabbitmq", label: "RabbitMQ", color: 0xfb923c, slot: portSlots.messaging },
  { id: "asynq", flag: "--asynq", label: "Asynq", color: 0xfacc15, slot: portSlots.jobs },
  { id: "river", flag: "--river", label: "River", color: 0x22d3ee, slot: portSlots.jobs },
  { id: "watermill", flag: "--watermill", label: "Watermill", color: 0xa5b4fc, slot: portSlots.jobs },
  { id: "keyvault", flag: "--key-vault", label: "Key Vault", color: 0x60a5fa, slot: portSlots.config },
];

export const frameworkColor: Record<Framework, number> = {
  gin: 0x38bdf8,
  echo: 0xc4b5fd,
  chi: 0xfdba74,
};

export const databaseColor: Record<Database, number> = {
  postgres: 0x60a5fa,
  mysql: 0xfbbf24,
  mongodb: 0x34d399,
  none: 0x64748b,
};

export interface Config {
  framework: Framework;
  database: Database;
  extras: string[];
}

// Same rules as normalizeFeatures: asynq turns on redis; river needs postgres;
// watermill needs kafka, rabbitmq or redis.
export function normalize(c: Config): Config {
  const on = new Set(c.extras);
  if (on.has("asynq")) on.add("redis");
  if (c.database !== "postgres") on.delete("river");
  if (on.has("watermill") && !["kafka", "rabbitmq", "redis"].some((k) => on.has(k))) {
    on.delete("watermill");
  }
  return { ...c, extras: extras.map((e) => e.id).filter((id) => on.has(id)) };
}

export const defaultName = "my-api";

// Same rule as ValidateName in internal/core/options.go.
const nameRe = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
export function validName(name: string): boolean {
  return nameRe.test(name);
}

export function command(c: Config, name = defaultName): string {
  const parts = ["krok new", name, `-f ${c.framework}`, `-d ${c.database}`];
  for (const e of extras) if (c.extras.includes(e.id)) parts.push(e.flag);
  return parts.join(" ");
}

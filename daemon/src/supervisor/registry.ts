// Worker registry — holds live WorkerHandle objects keyed by worker ID.
// The supervisor proper (spawn.ts) populates this; ctl + MCP layers read
// from it to answer queries.
import type { WorkerHandle } from "./spawn.ts";
import { log } from "../log.ts";

export class Supervisor {
  private workers = new Map<string, WorkerHandle>();
  /** Cap on simultaneously running workers (after counting state=running). */
  readonly maxConcurrent: number;

  constructor(maxConcurrent: number) {
    this.maxConcurrent = Math.max(1, maxConcurrent);
  }

  register(h: WorkerHandle): void {
    this.workers.set(h.id, h);
  }

  unregister(id: string): void {
    this.workers.delete(id);
  }

  get(id: string): WorkerHandle | undefined {
    return this.workers.get(id);
  }

  list(): WorkerHandle[] {
    return Array.from(this.workers.values());
  }

  /** Number of workers we believe are actively running. */
  runningCount(): number {
    return this.list().filter((h) => h.state === "running" || h.state === "starting").length;
  }

  /** Returns true if a new worker may be spawned right now. */
  canSpawn(): boolean {
    return this.runningCount() < this.maxConcurrent;
  }

  /** Best-effort: signal SIGTERM (via SDK abort) then SIGKILL after grace. */
  async stopAll(graceMs: number): Promise<void> {
    const handles = this.list();
    log.info("stopping all workers", { count: handles.length, graceMs });
    handles.forEach((h) => h.abort.abort("daemon-shutdown"));
    const deadline = Date.now() + graceMs;
    while (Date.now() < deadline && this.runningCount() > 0) {
      await new Promise((r) => setTimeout(r, 100));
    }
    // Anything still alive: nothing more we can do from here — the SDK
    // process should exit when aborted. v1 will add a hard kill path.
  }
}

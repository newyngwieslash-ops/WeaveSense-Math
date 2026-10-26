import { describe, expect, test } from "bun:test";
import { DEFAULT_CANVAS_AGENT_SKILL_ID, ensureDefaultCanvasAgentSkill } from "@/services/skill-defaults";

function skill(skillId: string, isAdded = true) {
    return { skillId, skillName: skillId, isAdded } as never;
}

describe("ensureDefaultCanvasAgentSkill", () => {
    test("does not install short-drama skills by default", async () => {
        let reads = 0;
        let writes = 0;
        const result = await ensureDefaultCanvasAgentSkill({
            listAdded: async () => ({ skills: [] }),
            get: async () => { reads++; return { skill: skill(DEFAULT_CANVAS_AGENT_SKILL_ID, false) }; },
            add: async () => { writes++; return { skill: skill(DEFAULT_CANVAS_AGENT_SKILL_ID) }; },
        });
        expect(reads).toBe(0);
        expect(writes).toBe(0);
        expect(result.skills).toEqual([]);
    });

    test("keeps historical skills with short drama disabled", async () => {
        const installed = [skill(DEFAULT_CANVAS_AGENT_SKILL_ID), skill("user-skill")];
        const result = await ensureDefaultCanvasAgentSkill({ listAdded: async () => ({ skills: installed }) }, false);
        expect(result.skills).toEqual(installed);
    });
    test("keeps an already added default skill without writing", async () => {
        let added = 0;
        const result = await ensureDefaultCanvasAgentSkill({
            listAdded: async () => ({ skills: [skill(DEFAULT_CANVAS_AGENT_SKILL_ID)] }),
            get: async () => {
                throw new Error("must not read catalog");
            },
            add: async () => {
                added += 1;
                return { skill: skill(DEFAULT_CANVAS_AGENT_SKILL_ID) };
            },
        }, true);
        expect(result.skills[0].skillId).toBe(DEFAULT_CANVAS_AGENT_SKILL_ID);
        expect(added).toBe(0);
    });

    test("installs the default skill once when it is not added", async () => {
        let listCalls = 0;
        let added = 0;
        const result = await ensureDefaultCanvasAgentSkill({
            listAdded: async () => ({ skills: listCalls++ === 0 ? [] : [skill(DEFAULT_CANVAS_AGENT_SKILL_ID)] }),
            get: async () => ({ skill: skill(DEFAULT_CANVAS_AGENT_SKILL_ID, false) }),
            add: async () => {
                added += 1;
                return { skill: skill(DEFAULT_CANVAS_AGENT_SKILL_ID) };
            },
        }, true);
        expect(result.skills[0].skillId).toBe(DEFAULT_CANVAS_AGENT_SKILL_ID);
        expect(added).toBe(1);
        expect(listCalls).toBe(2);
    });
});

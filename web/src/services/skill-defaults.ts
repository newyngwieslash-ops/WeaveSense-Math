import { addSkill, getSkill, listAddedSkills, type Skill } from "@/services/api/skills";

/** 显式开启短剧时的新画布默认技能；不覆盖历史会话选择。 */
export const DEFAULT_CANVAS_AGENT_SKILL_ID = "yingce-short-drama-workflow";

export type AddedSkillsResult = { skills: Skill[] };

/**
 * 确保默认技能既存在于用户技能库，也能作为本轮 Agent 的已选技能使用。
 * 先读已安装列表，只有首次需要时才写入用户技能状态。
 */
export async function ensureDefaultCanvasAgentSkill(
    dependencies: {
        listAdded?: typeof listAddedSkills;
        get?: typeof getSkill;
        add?: typeof addSkill;
    } = {},
    shortDramaEnabled = false,
): Promise<AddedSkillsResult> {
    const list = dependencies.listAdded || listAddedSkills;
    const get = dependencies.get || getSkill;
    const add = dependencies.add || addSkill;
    const added = await list();
    if (!shortDramaEnabled) return added;
    if (added.skills.some((skill) => skill.skillId === DEFAULT_CANVAS_AGENT_SKILL_ID && skill.isAdded)) return added;

    const detail = await get(DEFAULT_CANVAS_AGENT_SKILL_ID);
    if (!detail.skill.isAdded) await add(DEFAULT_CANVAS_AGENT_SKILL_ID);
    return list();
}

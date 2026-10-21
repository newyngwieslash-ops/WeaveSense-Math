import { expect, test } from "bun:test";
import { apiClient } from "../src/services/api/request";
import { removeMember } from "../src/services/api/intelligence";

test("member revoke uses workspace header, encoded ID and rejects business failure", async () => {
    const adapter = apiClient.defaults.adapter;
    let fail = false;
    try {
        apiClient.defaults.adapter = async (config) => {
            expect(config.method).toBe("delete");
            expect(config.url).toBe("/intelligence/members/user%2Ftwo");
            expect(config.headers.get("X-Workspace-Id")).toBe("ws_test");
            return { data: fail ? { code: 403, msg: "owner protected" } : { code: 0, data: null, msg: "ok" }, status: 200, statusText: "OK", headers: {}, config };
        };
        expect(await removeMember("ws_test", "user/two")).toBeNull();
        fail = true;
        await expect(removeMember("ws_test", "user/two")).rejects.toThrow("owner protected");
    } finally {
        apiClient.defaults.adapter = adapter;
    }
});

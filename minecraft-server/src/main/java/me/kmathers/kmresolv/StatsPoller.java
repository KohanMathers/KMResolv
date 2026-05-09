package me.kmathers.kmresolv;

import com.google.gson.JsonObject;

import net.kyori.adventure.text.Component;
import net.minestom.server.MinecraftServer;
import net.minestom.server.entity.Entity;
import net.minestom.server.entity.metadata.display.TextDisplayMeta;
import net.minestom.server.timer.TaskSchedule;

public class StatsPoller {

    public static void start(Entity tq, Entity chr, Entity bc, Entity al, Entity ut, Entity cs,
                              Entity tfm, Entity te, Entity tcf, Entity tp) {
        MinecraftServer.getSchedulerManager().buildTask(() -> {
            JsonObject stats = ApiClient.get("/api/stats");
            if (!stats.entrySet().isEmpty()) {
                long totalQueries  = stats.get("total_queries").getAsLong();
                double hitRate     = stats.get("hit_rate").getAsDouble();
                long blocked       = stats.get("blocked").getAsLong();
                double avgLatency  = stats.get("avg_latency_ms").getAsDouble();
                int uptimeSecs     = stats.get("uptime_seconds").getAsInt();
                int cacheSize      = stats.get("cache_size").getAsInt();
                int cacheNeg       = stats.get("cache_negative").getAsInt();

                int h = uptimeSecs / 3600;
                int m = (uptimeSecs % 3600) / 60;
                int s = uptimeSecs % 60;
                String uptime = String.format("%02d:%02d:%02d", h, m, s);

                tq.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Total Queries: ").append(Component.text(totalQueries))));
                chr.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Cache Hit Rate: ").append(Component.text(String.format("%.1f%%", hitRate)))));
                bc.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Blocked: ").append(Component.text(blocked))));
                al.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Avg Latency: ").append(Component.text(String.format("%.1fms", avgLatency)))));
                ut.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Uptime: ").append(Component.text(uptime))));
                cs.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Cache (P/N): ").append(Component.text(cacheSize + "/" + cacheNeg))));
            }

            JsonObject settings = ApiClient.get("/api/settings/get");
            if (!settings.entrySet().isEmpty()) {
                boolean edns0       = settings.get("edns0").getAsBoolean();
                boolean tcpFallback = settings.get("tcp_fallback").getAsBoolean();
                boolean prefetch    = settings.get("prefetch").getAsBoolean();
                @SuppressWarnings("unused")
                String  mode        = settings.get("log_level").getAsString();

                Server.setEd(edns0);
                Server.setTf(tcpFallback);
                Server.setPf(prefetch);

                te.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("EDNS0: ").append(
                        edns0 ? Component.text("On").color(net.kyori.adventure.text.format.NamedTextColor.GREEN)
                              : Component.text("Off").color(net.kyori.adventure.text.format.NamedTextColor.RED))));
                tcf.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("TCP: ").append(
                        tcpFallback ? Component.text("On").color(net.kyori.adventure.text.format.NamedTextColor.GREEN)
                                    : Component.text("Off").color(net.kyori.adventure.text.format.NamedTextColor.RED))));
                tp.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Prefetch: ").append(
                        prefetch ? Component.text("On").color(net.kyori.adventure.text.format.NamedTextColor.GREEN)
                                 : Component.text("Off").color(net.kyori.adventure.text.format.NamedTextColor.RED))));
            }

            JsonObject filter = ApiClient.get("/api/filter/status");
            if (!filter.entrySet().isEmpty()) {
                String mode = filter.get("mode").getAsString();
                String display = mode.substring(0, 1).toUpperCase() + mode.substring(1);
                FilterMode fm = switch (mode) {
                    case "whitelist" -> FilterMode.WHITELIST;
                    case "blacklist" -> FilterMode.BLACKLIST;
                    default -> FilterMode.OFF;
                };
                Server.setFm(fm);
                tfm.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
                    Component.text("Mode: ").append(Component.text(display))));
            }

        }).repeat(TaskSchedule.seconds(5)).schedule();
    }
}
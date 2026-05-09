package me.kmathers.kmresolv;

import net.kyori.adventure.text.Component;
import net.kyori.adventure.text.format.NamedTextColor;
import net.minestom.server.MinecraftServer;
import net.minestom.server.entity.Entity;
import net.minestom.server.entity.metadata.display.TextDisplayMeta;
import net.minestom.server.timer.TaskSchedule;

public class SettingHandler {

    public SettingHandler() {}

    public void handleFa(Entity fa) {
        ApiClient.post("/api/cache/flush?mode=all");
        fa.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("Flushed all cache.").color(NamedTextColor.RED)));
        MinecraftServer.getSchedulerManager().buildTask(() ->
            fa.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("")))
        ).delay(TaskSchedule.seconds(3)).schedule();
    }

    public void handleFe(Entity fe) {
        ApiClient.post("/api/cache/flush?mode=expired");
        fe.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("Flushed expired.").color(NamedTextColor.YELLOW)));
        MinecraftServer.getSchedulerManager().buildTask(() ->
            fe.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("")))
        ).delay(TaskSchedule.seconds(3)).schedule();
    }

    public void handleFn(Entity fn) {
        ApiClient.post("/api/cache/flush?mode=negative");
        fn.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("Flushed negative.").color(NamedTextColor.YELLOW)));
        MinecraftServer.getSchedulerManager().buildTask(() ->
            fn.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("")))
        ).delay(TaskSchedule.seconds(3)).schedule();
    }

    public void handleTfm(Entity tfm) {
        FilterMode fm = Server.getFm();
        String label = switch (fm) {
            case OFF -> { Server.setFm(FilterMode.WHITELIST); yield "whitelist"; }
            case WHITELIST -> { Server.setFm(FilterMode.BLACKLIST); yield "blacklist"; }
            case BLACKLIST -> { Server.setFm(FilterMode.OFF); yield "off"; }
        };
        ApiClient.post("/api/filter/mode", "{\"mode\":\"" + label + "\"}");
        String display = label.substring(0, 1).toUpperCase() + label.substring(1);
        tfm.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("Mode: ").append(Component.text(display))));
    }

    public void handleTe(Entity te) {
        boolean nowOn = !Server.getEd();
        Server.setEd(nowOn);
        ApiClient.post("/api/settings", "{\"edns0\":" + nowOn + "}");
        Component c = nowOn
            ? Component.text("On").color(NamedTextColor.GREEN)
            : Component.text("Off").color(NamedTextColor.RED);
        te.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("EDNS0: ").append(c)));
    }

    public void handleTcf(Entity tcf) {
        boolean nowOn = !Server.getTf();
        Server.setTf(nowOn);
        ApiClient.post("/api/settings", "{\"tcp_fallback\":" + nowOn + "}");
        Component c = nowOn
            ? Component.text("On").color(NamedTextColor.GREEN)
            : Component.text("Off").color(NamedTextColor.RED);
        tcf.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("TCP: ").append(c)));
    }

    public void handleTp(Entity tp) {
        boolean nowOn = !Server.getPf();
        Server.setPf(nowOn);
        ApiClient.post("/api/settings", "{\"prefetch\":" + nowOn + "}");
        Component c = nowOn
            ? Component.text("On").color(NamedTextColor.GREEN)
            : Component.text("Off").color(NamedTextColor.RED);
        tp.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(
            Component.text("Prefetch: ").append(c)));
    }
}
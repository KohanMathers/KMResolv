package me.kmathers.kmresolv;

import java.io.IOException;
import java.nio.file.Path;
import java.util.HashMap;

import net.hollowcube.polar.PolarLoader;
import net.kyori.adventure.text.Component;
import net.kyori.adventure.text.format.NamedTextColor;
import net.minestom.server.Auth;
import net.minestom.server.MinecraftServer;
import net.minestom.server.coordinate.Pos;
import net.minestom.server.entity.Entity;
import net.minestom.server.entity.EntityType;
import net.minestom.server.entity.GameMode;
import net.minestom.server.entity.metadata.display.TextDisplayMeta;
import net.minestom.server.event.GlobalEventHandler;
import net.minestom.server.event.player.AsyncPlayerConfigurationEvent;
import net.minestom.server.event.player.PlayerBlockInteractEvent;
import net.minestom.server.instance.InstanceContainer;
import net.minestom.server.instance.InstanceManager;
import net.minestom.server.instance.block.Block;
import net.minestom.server.instance.block.BlockManager;

public class Server {

    static FilterMode fm = FilterMode.OFF;
    static Boolean ed = false;
    static Boolean tf = false;
    static Boolean pf = false;

    static SettingHandler sh = new SettingHandler();

    public static void main(String[] args) throws IOException {
        String addr = "0.0.0.0";
        int port = 25565;

        HashMap<Pos,Setting> hmps = new HashMap();
        hmps.put(new Pos(12, -59, -7), Setting.FLUSH_ALL);
        hmps.put(new Pos(12, -59, -5), Setting.FLUSH_EXPIRED);
        hmps.put(new Pos(12, -59, -3), Setting.FLUSH_NEGATIVE);
        hmps.put(new Pos(10, -59, 2), Setting.TOGGLE_FILTER_MODE);
        hmps.put(new Pos(8, -59, 2), Setting.TOGGLE_EDNS0);
        hmps.put(new Pos(2, -59, 2), Setting.TOGGLE_TCP_FALLBACK);
        hmps.put(new Pos(0, -59, 2), Setting.TOGGLE_PREFETCH);

        Entity ct = new Entity(EntityType.TEXT_DISPLAY);
        Entity fa = new Entity(EntityType.TEXT_DISPLAY);
        Entity fe = new Entity(EntityType.TEXT_DISPLAY);
        Entity fn = new Entity(EntityType.TEXT_DISPLAY);
        Entity ts1 = new Entity(EntityType.TEXT_DISPLAY);
        Entity tfm = new Entity(EntityType.TEXT_DISPLAY);
        Entity te = new Entity(EntityType.TEXT_DISPLAY);
        Entity ts2 = new Entity(EntityType.TEXT_DISPLAY);
        Entity tcf = new Entity(EntityType.TEXT_DISPLAY);
        Entity tp = new Entity(EntityType.TEXT_DISPLAY);
        Entity tq = new Entity(EntityType.TEXT_DISPLAY);
        Entity chr = new Entity(EntityType.TEXT_DISPLAY);
        Entity bc = new Entity(EntityType.TEXT_DISPLAY);
        Entity al = new Entity(EntityType.TEXT_DISPLAY);
        Entity ut = new Entity(EntityType.TEXT_DISPLAY);
        Entity cs = new Entity(EntityType.TEXT_DISPLAY);

        for (int i = 0; i < args.length - 1; i++) {
            switch (args[i]) {
                case "--addr", "-a" -> addr = args[++i];
                case "--port", "-p" -> port = Integer.parseInt(args[++i]);
                case "--api", "-api" -> ApiClient.setBaseUrl(args[++i]);
            }
        }

        MinecraftServer server = MinecraftServer.init(new Auth.Offline());

        InstanceManager im = MinecraftServer.getInstanceManager();
        InstanceContainer ic = im.createInstanceContainer();

        ic.setChunkLoader(new PolarLoader(Path.of("kmresolv.polar")));

        BlockManager bm = MinecraftServer.getBlockManager();
        bm.registerHandler("minecraft:sign", () -> SignHandler.INSTANCE);

        spawnTextDisplays(ct, fa, fe, fn, ts1, tfm, te, ts2, tcf, tp, tq, chr, bc, al, ut, cs, ic);

        StatsPoller.start(tq, chr, bc, al, ut, cs, tfm, te, tcf, tp);

        GlobalEventHandler geh = MinecraftServer.getGlobalEventHandler();

        geh.addListener(AsyncPlayerConfigurationEvent.class, event -> {
            event.setSpawningInstance(ic);
            event.getPlayer().setRespawnPoint(new Pos(5.5, -58, -4.5));
            event.getPlayer().setGameMode(GameMode.ADVENTURE);
        });

        geh.addListener(PlayerBlockInteractEvent.class, event -> {
            event.setCancelled(true);
            event.getPlayer().getInventory().clear();
            Block b = event.getBlock();
            Pos bp = event.getBlockPosition().asPos();
            if (b.compare(Block.STONE_BUTTON)) {
                if(hmps.containsKey(bp)) {
                    Setting s = hmps.get(bp);
                    if (null != s) switch (s) {
                        case FLUSH_ALL -> sh.handleFa(fa);
                        case FLUSH_EXPIRED -> sh.handleFe(fe);
                        case FLUSH_NEGATIVE -> sh.handleFn(fn);
                        case TOGGLE_FILTER_MODE -> sh.handleTfm(tfm);
                        case TOGGLE_EDNS0 -> sh.handleTe(te);
                        case TOGGLE_TCP_FALLBACK -> sh.handleTcf(tcf);
                        case TOGGLE_PREFETCH -> sh.handleTp(tp);
                        default -> {
                        }
                    }
                }
            }
        });

        server.start(addr, port);
    }

    private static void spawnTextDisplays(Entity ct, Entity fa, Entity fe, Entity fn, Entity ts1, Entity tfm, Entity te, Entity ts2, Entity tcf, Entity tp, Entity tq, Entity chr, Entity bc, Entity al, Entity ut, Entity cs, InstanceContainer ic) {
        ct.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Cache Settings")));
        ct.setNoGravity(true);
        ct.setInstance(ic, new Pos(13, -57, -4.5, 90, 0));

        fa.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("")));
        fa.setNoGravity(true);
        fa.setInstance(ic, new Pos(12.5, -58, -6.5, 90, 0));

        fe.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("")));
        fe.setNoGravity(true);
        fe.setInstance(ic, new Pos(12.5, -58, -4.5, 90, 0));

        fn.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("")));
        fn.setNoGravity(true);
        fn.setInstance(ic, new Pos(12.5, -58, -2.5, 90, 0));

        ts1.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Toggle Settings")));
        ts1.setNoGravity(true);
        ts1.setInstance(ic, new Pos(9.5, -57, 3, 180, 0));

        // Needs updating to fetch from api
        tfm.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Mode: ").append(Component.text("N/A"))));
        tfm.setNoGravity(true);
        tfm.setInstance(ic, new Pos(10.5, -58, 2.5, 180, 0));

        // Needs updating to fetch from api
        te.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("EDNS0: ").append(Component.text("Off").color(NamedTextColor.RED))));
        te.setNoGravity(true);
        te.setInstance(ic, new Pos(8.5, -58, 2.5, 180, 0));

        ts2.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Toggle Settings")));
        ts2.setNoGravity(true);
        ts2.setInstance(ic, new Pos(1.5, -57, 3, 180, 0));

        // Needs updating to fetch from api
        tcf.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("TCP: ").append(Component.text("Off").color(NamedTextColor.RED))));
        tcf.setNoGravity(true);
        tcf.setInstance(ic, new Pos(2.5, -58, 2.5, 180, 0));

        // Needs updating to fetch from api
        tp.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Prefetch: ").append(Component.text("Off").color(NamedTextColor.RED))));
        tp.setNoGravity(true);
        tp.setInstance(ic, new Pos(0.5, -58, 2.5, 180, 0));

        // Needs updating to fetch from api
        tq.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Total Queries: ").append(Component.text("0"))));
        tq.setNoGravity(true);
        tq.setInstance(ic, new Pos(-2, -56.5, -2.5, -90, 0));

        // Needs updating to fetch from api
        chr.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Cache Hit Rate: ").append(Component.text("0%"))));
        chr.setNoGravity(true);
        chr.setInstance(ic, new Pos(-2, -57.5, -2.5, -90, 0));

        // Needs updating to fetch from api
        bc.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Blocked Count: ").append(Component.text("0"))));
        bc.setNoGravity(true);
        bc.setInstance(ic, new Pos(-2, -58.5, -2.5, -90, 0));

        // Needs updating to fetch from api
        al.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Avg Latency: ").append(Component.text("0ms"))));
        al.setNoGravity(true);
        al.setInstance(ic, new Pos(-2, -56.5, -6.5, -90, 0));

        // Needs updating to fetch from api
        ut.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Uptime: ").append(Component.text("0:0:0"))));
        ut.setNoGravity(true);
        ut.setInstance(ic, new Pos(-2, -57.5, -6.5, -90, 0));

        // Needs updating to fetch from api
        cs.editEntityMeta(TextDisplayMeta.class, meta -> meta.setText(Component.text("Cache Size (P/N): ").append(Component.text("0/0"))));
        cs.setNoGravity(true);
        cs.setInstance(ic, new Pos(-2, -58.5, -6.5, -90, 0));
    }

    public static FilterMode getFm() {
        return fm;
    }

    public static Boolean getEd() {
        return ed;
    }

    public static Boolean getTf() {
        return tf;
    }

    public static Boolean getPf() {
        return pf;
    }

    public static void setFm(FilterMode fm) {
        Server.fm = fm;
    }

    public static void setEd(Boolean ed) {
        Server.ed = ed;
    }

    public static void setTf(Boolean tf) {
        Server.tf = tf;
    }

    public static void setPf(Boolean pf) {
        Server.pf = pf;
    }
}

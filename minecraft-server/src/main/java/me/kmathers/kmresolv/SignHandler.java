package me.kmathers.kmresolv;

import java.util.Collection;
import java.util.List;

import net.kyori.adventure.key.Key;
import net.minestom.server.instance.block.BlockHandler;
import net.minestom.server.tag.Tag;
import org.jetbrains.annotations.NotNull;

public class SignHandler implements BlockHandler {
    public static final SignHandler INSTANCE = new SignHandler();

    @Override
    public @NotNull Key getKey() {
        return Key.key("minecraft:sign");
    }

    @Override
    public byte getBlockEntityAction() {
        return 9;
    }

    @Override
    public @NotNull Collection<Tag<?>> getBlockEntityTags() {
        return List.of(
            Tag.NBT("front_text"),
            Tag.NBT("back_text"),
            Tag.Boolean("is_waxed")
        );
    }
}

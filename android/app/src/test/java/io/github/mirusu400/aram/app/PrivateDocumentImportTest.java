package io.github.mirusu400.aram.app;

import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;

import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

import static org.junit.Assert.*;

public class PrivateDocumentImportTest {
    @Rule
    public TemporaryFolder temporary = new TemporaryFolder();

    @Test
    public void completedImportSurvivesProcessDeathAndExpiredUriPermission() throws Exception {
        File root = temporary.newFolder();
        byte[] data = {1, 2, 3};
        PrivateDocumentImport.Result first = PrivateDocumentImport.copy(
                root, "request", source(new ByteArrayInputStream(data))
        );
        PrivateDocumentImport.Result restored = PrivateDocumentImport.copy(
                root, "request", new PrivateDocumentImport.Source() {
                    public String displayName() { throw new AssertionError("URI grant expired"); }
                    public InputStream open() { throw new AssertionError("must reuse completed file"); }
                }
        );
        assertEquals(first.file, restored.file);
        assertArrayEquals(data, Files.readAllBytes(restored.file.toPath()));
        assertEquals(1, root.listFiles().length);
    }

    @Test
    public void interruptedImportRetriesWithoutExposingPartialBytes() throws Exception {
        File root = temporary.newFolder();
        InputStream broken = new InputStream() {
            public int read() throws IOException { throw new IOException("provider interrupted"); }
        };
        assertThrows(IOException.class, () -> PrivateDocumentImport.copy(root, "request", source(broken)));
        assertEquals(0, new File(root, "request").listFiles().length);
        // Also simulate process death leaving an unfinished temporary file.
        Files.write(new File(root, "request.part").toPath(), new byte[]{9, 9, 9, 9});
        PrivateDocumentImport.Result result = PrivateDocumentImport.copy(
                root, "request", source(new ByteArrayInputStream(new byte[]{1, 2}))
        );
        assertArrayEquals(new byte[]{1, 2}, Files.readAllBytes(result.file.toPath()));
        assertFalse(new File(root, "request.part").exists());
        assertEquals(1, root.listFiles().length);
    }

    @Test
    public void providerCloseFailureDoesNotPublishACompletedImport() throws Exception {
        File root = temporary.newFolder();
        InputStream broken = new ByteArrayInputStream(new byte[]{1, 2}) {
            public void close() throws IOException { throw new IOException("close failed"); }
        };
        assertThrows(IOException.class, () -> PrivateDocumentImport.copy(root, "request", source(broken)));
        assertEquals(0, new File(root, "request").listFiles().length);
        assertFalse(new File(root, "request.part").exists());
    }

    private static PrivateDocumentImport.Source source(InputStream stream) {
        return new PrivateDocumentImport.Source() {
            public String displayName() { return "synthetic.zip"; }
            public InputStream open() { return stream; }
        };
    }
}

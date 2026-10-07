package io.github.mirusu400.aram.app;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;

/** An import ID stays stable while its Activity or process is recreated. */
final class PrivateDocumentImport {
    private static final long MAX_IMPORT_BYTES = 2L * 1024L * 1024L * 1024L;

    interface Source {
        String displayName();
        InputStream open() throws IOException;
    }

    static final class Result {
        final File file;
        final String displayName;

        Result(File file, String displayName) {
            this.file = file;
            this.displayName = displayName;
        }
    }

    static Result copy(File root, String requestId, Source source) throws IOException {
        File imports = new File(root, requestId);
        if (!imports.isDirectory() && !imports.mkdirs()) {
            throw new IOException("cannot create the import directory");
        }
        // The temporary file is outside this directory. A completed file is
        // visible here only after all provider closes, syncing and rename succeed.
        File[] completed = imports.listFiles(File::isFile);
        if (completed != null && completed.length == 1) {
            return new Result(completed[0], completed[0].getName());
        }
        String displayName = source.displayName();
        File destination = new File(imports, safeFileName(displayName));
        File temporary = new File(root, requestId + ".part");

        long total = 0;
        try (
                InputStream raw = source.open();
                BufferedInputStream input = raw == null ? null : new BufferedInputStream(raw);
                FileOutputStream fileOutput = new FileOutputStream(temporary);
                BufferedOutputStream output = new BufferedOutputStream(fileOutput)
        ) {
            if (input == null) {
                throw new IOException("the document provider returned no data");
            }
            byte[] buffer = new byte[64 * 1024];
            for (int count; (count = input.read(buffer)) != -1; ) {
                total += count;
                if (total > MAX_IMPORT_BYTES) {
                    throw new IOException("the selected document exceeds 2 GiB");
                }
                output.write(buffer, 0, count);
            }
            output.flush();
            fileOutput.getFD().sync();
        } catch (IOException | RuntimeException error) {
            temporary.delete();
            throw error;
        }

        if (!temporary.renameTo(destination)) {
            temporary.delete();
            throw new IOException("cannot finish the imported document");
        }
        return new Result(destination, displayName);
    }

    private static String safeFileName(String name) {
        String result = name.replaceAll("[\\\\/:*?\"<>|\\p{Cntrl}]", "_").trim();
        if (result.isEmpty() || ".".equals(result) || "..".equals(result)) {
            result = "document";
        }
        if (result.length() > 120) {
            result = result.substring(result.length() - 120);
        }
        return result;
    }
}

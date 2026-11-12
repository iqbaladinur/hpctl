package id.iqbaldev.hpctl.companion

import android.accessibilityservice.AccessibilityService
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Rect
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import androidx.core.content.ContextCompat
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import android.view.accessibility.AccessibilityWindowInfo
import java.io.File

/**
 * Keeps a JSON copy of the current screen's element tree on disk, refreshed a
 * few times a second as the screen changes. The computer reads that file with
 * one `adb exec-out cat` instead of paying for a fresh uiautomator dump.
 *
 * The file is written to the app's external files dir, which adb shell can
 * read, and mirrored to the internal one so `run-as` works as a fallback.
 */
class TreeDumpService : AccessibilityService() {

    private val handler = Handler(Looper.getMainLooper())
    private var scheduled = false
    private var seq = 0L
    private var receiver: BroadcastReceiver? = null

    /**
     * `am broadcast -a id.iqbaldev.hpctl.SET_TEXT --es text "..."` writes text
     * straight into a field with ACTION_SET_TEXT: any script, no keyboard, and
     * the clipboard is left alone. The answer goes to set_result.json because a
     * broadcast cannot return a value.
     */
    private val setTextReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent == null) return
            val req = intent.getStringExtra("req").orEmpty()
            // `am` drops an empty --es text, so clearing is its own flag.
            val text = if (intent.getBooleanExtra("clear", false)) "" else intent.getStringExtra("text").orEmpty()
            val into = intent.getStringExtra("into").orEmpty()
            val append = intent.getBooleanExtra("append", false)

            val node = findEditable(into)
            var ok = false
            var err = ""
            if (node == null) {
                err = if (into.isEmpty()) "no focused editable field" else "no editable field matching \"$into\""
            } else {
                val cur = node.text?.toString().orEmpty()
                val b = Bundle()
                b.putCharSequence(
                    AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE,
                    if (append) cur + text else text,
                )
                ok = node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, b)
                if (!ok) err = "ACTION_SET_TEXT was refused"
            }
            writeNamed("set_result.json", "{\"req\":\"" + esc(req) + "\",\"ok\":" + ok +
                ",\"error\":\"" + esc(err) + "\"}")
            dump()
        }
    }

    private fun findEditable(want: String): AccessibilityNodeInfo? {
        val editable = ArrayList<AccessibilityNodeInfo>()
        for (w in windows) {
            if (w.type != AccessibilityWindowInfo.TYPE_APPLICATION) continue
            val root = try {
                w.root
            } catch (e: Exception) {
                null
            } ?: continue
            collectEditable(root, editable)
        }
        if (editable.isEmpty()) return null
        if (want.isNotEmpty()) {
            val w = want.lowercase()
            val matches = editable.filter { n ->
                n.viewIdResourceName.orEmpty().lowercase().contains(w) ||
                    n.text?.toString().orEmpty().lowercase().contains(w) ||
                    n.contentDescription?.toString().orEmpty().lowercase().contains(w) ||
                    n.hintText?.toString().orEmpty().lowercase().contains(w)
            }
            // A screen can hold two fields with the same name (Settings keeps a
            // collapsed search bar beside the open one); the one being typed
            // into is the one meant.
            (matches.firstOrNull { it.isFocused } ?: matches.firstOrNull())?.let { return it }
        }
        return editable.firstOrNull { it.isFocused } ?: editable.first()
    }

    private fun collectEditable(n: AccessibilityNodeInfo, out: MutableList<AccessibilityNodeInfo>) {
        if (n.isEditable) out.add(n)
        for (i in 0 until n.childCount) {
            val c = try {
                n.getChild(i)
            } catch (e: Exception) {
                null
            } ?: continue
            collectEditable(c, out)
        }
    }

    override fun onServiceConnected() {
        super.onServiceConnected()
        val r = setTextReceiver
        receiver = r
        ContextCompat.registerReceiver(
            this, r, IntentFilter(ACTION_SET_TEXT), ContextCompat.RECEIVER_EXPORTED,
        )
        dump()
        // A screen that stops changing stops raising events, and a file that
        // stops refreshing is one the computer cannot trust. The tick keeps it
        // current; one in-process walk costs a few milliseconds.
        handler.postDelayed(object : Runnable {
            override fun run() {
                dump()
                handler.postDelayed(this, TICK_MS)
            }
        }, TICK_MS)
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (scheduled) return
        scheduled = true
        handler.postDelayed({
            scheduled = false
            dump()
        }, DEBOUNCE_MS)
    }

    override fun onInterrupt() = Unit

    override fun onDestroy() {
        receiver?.let {
            try {
                unregisterReceiver(it)
            } catch (e: Exception) {
                // Already gone.
            }
        }
        receiver = null
        super.onDestroy()
    }

    private fun dump() {
        val sb = StringBuilder(32 * 1024)
        seq++
        sb.append("{\"seq\":").append(seq)
        sb.append(",\"ts\":").append(System.currentTimeMillis())

        var pkg = ""
        var first = true
        sb.append(",\"nodes\":[")
        // Only application windows: the status and navigation bars belong to
        // no app and would bury the real screen under system chrome.
        for (w in windows) {
            if (w.type != AccessibilityWindowInfo.TYPE_APPLICATION) continue
            val root = try {
                w.root
            } catch (e: Exception) {
                null
            } ?: continue
            val p = root.packageName?.toString().orEmpty()
            if (first || w.isActive) {
                pkg = p
                first = false
            }
            appendNode(sb, root)
        }
        if (sb.last() == ',') sb.setLength(sb.length - 1)
        sb.append("],\"pkg\":\"").append(esc(pkg)).append("\"")

        val dm = resources.displayMetrics
        sb.append(",\"w\":").append(dm.widthPixels)
        sb.append(",\"h\":").append(dm.heightPixels)
        sb.append("}")

        writeNamed(FILE, sb.toString())
    }

    private fun appendNode(sb: StringBuilder, root: AccessibilityNodeInfo) {
        val stack = ArrayDeque<AccessibilityNodeInfo>()
        stack.addLast(root)
        while (stack.isNotEmpty()) {
            val n = stack.removeLast()
            val r = Rect()
            n.getBoundsInScreen(r)
            if (r.width() > 0 && r.height() > 0 && sb.length < MAX_JSON) {
                sb.append("{\"c\":\"").append(esc(simpleClass(n.className))).append('"')
                val text = n.text?.toString().orEmpty()
                if (text.isNotEmpty()) sb.append(",\"t\":\"").append(esc(text)).append('"')
                val desc = n.contentDescription?.toString().orEmpty()
                if (desc.isNotEmpty()) sb.append(",\"d\":\"").append(esc(desc)).append('"')
                val id = n.viewIdResourceName.orEmpty()
                if (id.isNotEmpty()) sb.append(",\"i\":\"").append(esc(id)).append('"')
                sb.append(",\"b\":[").append(r.left).append(',').append(r.top).append(',')
                    .append(r.right).append(',').append(r.bottom).append(']')
                if (n.isClickable) sb.append(",\"cl\":1")
                if (!n.isEnabled) sb.append(",\"dis\":1")
                if (n.isFocused) sb.append(",\"f\":1")
                if (n.isScrollable) sb.append(",\"sc\":1")
                if (n.isChecked) sb.append(",\"ch\":1")
                sb.append("},")
            }

            // Children are queued even when the parent draws nothing: a zero
            // sized container can still hold visible buttons.
            for (i in n.childCount - 1 downTo 0) {
                val c = try {
                    n.getChild(i)
                } catch (e: Exception) {
                    null
                } ?: continue
                stack.addLast(c)
            }
        }
    }

    private fun simpleClass(c: CharSequence?): String {
        val s = c?.toString().orEmpty()
        if (s.isEmpty()) return ""
        return s.substringAfterLast('.')
    }

    private fun esc(s: String): String {
        val b = StringBuilder(s.length + 8)
        for (ch in s) {
            when (ch) {
                '"' -> b.append("\\\"")
                '\\' -> b.append("\\\\")
                '\n' -> b.append("\\n")
                '\r' -> b.append("\\r")
                '\t' -> b.append("\\t")
                else -> if (ch < ' ') b.append(' ') else b.append(ch)
            }
        }
        return b.toString()
    }

    private fun writeNamed(name: String, json: String) {
        for (f in listOf(File(externalFile().parentFile, name), File(internalFile().parentFile, name))) {
            try {
                val tmp = File(f.parentFile, f.name + ".tmp")
                tmp.writeText(json)
                if (!tmp.renameTo(f)) {
                    f.writeText(json)
                    tmp.delete()
                }
            } catch (e: Exception) {
                // One of the two targets failing must not stop the other.
            }
        }
    }

    private fun externalFile(): File {
        val dir = getExternalFilesDir(null) ?: File("/sdcard/hpctl-companion")
        dir.mkdirs()
        return File(dir, FILE)
    }

    private fun internalFile(): File {
        val dir = File(filesDir, "pub")
        dir.mkdirs()
        return File(dir, FILE)
    }

    companion object {
        private const val DEBOUNCE_MS = 120L
        private const val TICK_MS = 900L
        private const val MAX_JSON = 200_000
        const val FILE = "tree.json"
        const val ACTION_SET_TEXT = "id.iqbaldev.hpctl.SET_TEXT"
    }
}
